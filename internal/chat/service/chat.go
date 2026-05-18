package service

import (
	"encoding/json"
	"log"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"recallix/internal/auth"
	"recallix/internal/chat/prompt"
	"recallix/internal/chat/query"
	"recallix/internal/config"
	"recallix/internal/memory"
	"recallix/internal/model/llm"
	"recallix/internal/repository"
	"recallix/internal/retrieval/hybrid"
	"recallix/internal/shared"
	"recallix/internal/task"
	"recallix/internal/types"
)

type ChatService struct {
	db      *gorm.DB
	cfg     *config.Config
	chat    *llm.ChatClient
	embed   *llm.EmbeddingClient
	rerank  *llm.RerankClient
	hybrid  *hybrid.Service
	memory  *memory.Service
	taskCli *asynq.Client
}

func New(db *gorm.DB, cfg *config.Config, chat *llm.ChatClient, embed *llm.EmbeddingClient,
	rerank *llm.RerankClient, hybrid *hybrid.Service, mem *memory.Service, taskCli *asynq.Client) *ChatService {
	return &ChatService{
		db: db, cfg: cfg, chat: chat, embed: embed,
		rerank: rerank, hybrid: hybrid, memory: mem, taskCli: taskCli,
	}
}

func (s *ChatService) Chat(c *gin.Context) {
	userID := auth.GetUserID(c)
	sessionID := c.Param("id")

	var session repository.Session
	if err := s.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		s.send(c, types.StreamResponse{ResponseType: types.ResponseTypeError, Content: "Session not found", Done: true})
		return
	}

	var req struct {
		Question string `json:"question" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.send(c, types.StreamResponse{ResponseType: types.ResponseTypeError, Content: "Invalid request", Done: true})
		return
	}

	// Save user message
	userMsg := repository.Message{
		ID:        shared.NewID(),
		SessionID: sessionID,
		Role:      "user",
		Content:   req.Question,
		CreatedAt: time.Now(),
	}
	s.db.Create(&userMsg)

	// Load history
	var recentMessages []repository.Message
	s.db.Where("session_id = ?", sessionID).Order("created_at desc").Limit(20).Find(&recentMessages)

	history := make([]llm.ChatMessage, 0)
	for i := len(recentMessages) - 1; i >= 0; i-- {
		history = append(history, llm.ChatMessage{
			Role:    recentMessages[i].Role,
			Content: recentMessages[i].Content,
		})
	}

	// Query understanding: rewrite + intent classification.
	understood := query.Understand(s.chat, history, req.Question)
	rewriteQuery := understood.RewriteQuery
	log.Printf("[Chat] QueryUnderstanding: query=%q rewrite=%q intent=%s needs_retrieval=%t",
		req.Question, rewriteQuery, understood.Intent, understood.NeedsRetrieval())

	// Compute the query embedding once per turn and reuse it for both memory
	// recall and knowledge retrieval.
	queryEmbedding, queryEmbeddingErr := s.embed.Embed(rewriteQuery)
	if queryEmbeddingErr != nil {
		log.Printf("[Chat] Query embedding error: %v", queryEmbeddingErr)
	}

	// Recall memories
	var memories []repository.Memory
	if queryEmbeddingErr == nil {
		memories, _ = s.memory.RecallWithEmbedding(userID, queryEmbedding, 3)
	}
	memContext := prompt.BuildMemoryContext(memories)

	// Knowledge search
	var kbID string
	var kbs []repository.KnowledgeBase
	if s.db.Where("user_id = ?", userID).Find(&kbs); len(kbs) > 0 {
		kbID = kbs[0].ID
	}

	var hybridResults []hybrid.Result
	if !understood.NeedsRetrieval() {
		log.Printf("[Chat] RAG bypass: intent=%s query=%q", understood.Intent, req.Question)
	} else if queryEmbeddingErr != nil {
		log.Printf("[Chat] Search skipped because query embedding failed: %v", queryEmbeddingErr)
	} else if s.hybrid != nil {
		results, err := s.hybrid.SearchWithEmbedding(userID, kbID, rewriteQuery, queryEmbedding, 10)
		if err != nil {
			log.Printf("[Chat] Search error: %v", err)
		} else {
			hybridResults = results
		}
	}

	// Rerank
	if len(hybridResults) > 0 && s.rerank != nil {
		docs := make([]string, len(hybridResults))
		for i, r := range hybridResults {
			docs[i] = r.Content
		}
		scores, err := s.rerank.Rerank(rewriteQuery, docs)
		if err == nil {
			for i := range hybridResults {
				if i < len(scores) {
					hybridResults[i].Score = scores[i]
				}
			}
		}
	}

	hybridResults = filterRAGResults(rewriteQuery, hybridResults, ragFilterOptions{
		TopK:               s.cfg.RAGTopK,
		ScoreThreshold:     s.cfg.RAGScoreThreshold,
		FallbackMinScore:   s.cfg.RAGFallbackMinScore,
		ThresholdFloor:     s.cfg.RAGThresholdFloor,
		ThresholdDegradeBy: s.cfg.RAGThresholdDegradeBy,
	})

	if len(hybridResults) == 0 {
		log.Printf("[Chat] RAG miss: query=%q", rewriteQuery)
	} else {
		log.Printf("[Chat] RAG hit: query=%q results=%d", rewriteQuery, len(hybridResults))
		for i, r := range hybridResults {
			log.Printf("[Chat] RAG[%d] chunk=%s knowledge=%s seq=%d score=%.4f",
				i+1, r.ChunkID, r.KnowledgeID, r.Seq, r.Score)
		}
	}

	// Build context
	chunks := make([]repository.Chunk, len(hybridResults))
	for i, r := range hybridResults {
		chunks[i] = repository.Chunk{Content: r.Content, ContextHeader: r.ContextHeader, Seq: r.Seq}
	}

	// Build messages
	systemPrompt := prompt.BuildSystemPrompt()
	if !understood.NeedsRetrieval() {
		systemPrompt = prompt.BuildNoRetrievalSystemPrompt(string(understood.Intent))
	}
	messages := []llm.ChatMessage{{Role: "system", Content: systemPrompt}}
	if memContext != "" {
		messages = append(messages, llm.ChatMessage{Role: "system", Content: memContext})
	}
	contextText := prompt.BuildContext(chunks)
	if contextText != "" {
		messages = append(messages, llm.ChatMessage{Role: "system", Content: contextText})
	}
	for _, msg := range history {
		if msg.Role == "user" || msg.Role == "assistant" {
			messages = append(messages, msg)
		}
	}

	// Send references
	retrievalStatus := types.RetrievalStatusMiss
	if !understood.NeedsRetrieval() {
		retrievalStatus = types.RetrievalStatusSkipped
	} else if len(hybridResults) > 0 {
		retrievalStatus = types.RetrievalStatusHit
	}
	refsJSON, _ := json.Marshal(hybridResults)
	s.send(c, types.StreamResponse{
		ResponseType:    types.ResponseTypeReferences,
		RetrievalStatus: retrievalStatus,
		References:      json.RawMessage(refsJSON),
	})

	// Stream answer
	fullAnswer, err := s.chat.ChatStream(messages, func(delta string) error {
		s.send(c, types.StreamResponse{
			ResponseType: types.ResponseTypeAnswer,
			Content:      delta,
		})
		return nil
	})

	if err != nil {
		log.Printf("[Chat] Stream error: %v", err)
		s.send(c, types.StreamResponse{ResponseType: types.ResponseTypeError, Content: err.Error(), Done: true})
		return
	}

	// Save assistant message
	assistantMsg := repository.Message{
		ID:              shared.NewID(),
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         fullAnswer,
		RetrievalStatus: string(retrievalStatus),
		CreatedAt:       time.Now(),
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&assistantMsg).Error; err != nil {
			return err
		}
		if len(hybridResults) == 0 {
			return nil
		}
		references := make([]repository.MessageReference, 0, len(hybridResults))
		for i, result := range hybridResults {
			references = append(references, repository.MessageReference{
				ID:                    shared.NewID(),
				MessageID:             assistantMsg.ID,
				ChunkID:               result.ChunkID,
				KnowledgeID:           result.KnowledgeID,
				KnowledgeBaseID:       result.KnowledgeBaseID,
				Rank:                  i + 1,
				Score:                 result.Score,
				Seq:                   result.Seq,
				ContextHeaderSnapshot: result.ContextHeader,
				ContentSnapshot:       result.Content,
				CreatedAt:             time.Now(),
			})
		}
		return tx.Create(&references).Error
	}); err != nil {
		log.Printf("[Chat] Failed to save assistant message with references: %v", err)
	}

	// Update session
	session.UpdatedAt = time.Now()
	if session.Title == "新对话" && len(req.Question) > 0 {
		title := []rune(req.Question)
		if len(title) > 50 {
			title = title[:50]
		}
		session.Title = string(title)
	}
	s.db.Save(&session)

	// Done
	s.send(c, types.StreamResponse{ResponseType: types.ResponseTypeStop, Done: true})

	// Async memory extraction
	memTask, err := task.NewMemoryExtractTask(userID, sessionID, req.Question, fullAnswer)
	if err != nil {
		log.Printf("[Chat] Failed to create memory extraction task: %v", err)
	} else if _, err := s.taskCli.Enqueue(memTask); err != nil {
		log.Printf("[Chat] Failed to enqueue memory extraction task: %v", err)
	}
}

func (s *ChatService) send(c *gin.Context, resp types.StreamResponse) {
	c.SSEvent("message", resp)
	c.Writer.Flush()
}

type ragFilterOptions struct {
	TopK               int
	ScoreThreshold     float64
	FallbackMinScore   float64
	ThresholdFloor     float64
	ThresholdDegradeBy float64
}

func filterRAGResults(queryText string, results []hybrid.Result, opts ragFilterOptions) []hybrid.Result {
	if len(results) == 0 {
		return nil
	}
	opts = opts.withDefaults()

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if opts.TopK > 0 && len(results) > opts.TopK {
		results = results[:opts.TopK]
	}

	filterAt := func(threshold float64) []hybrid.Result {
		filtered := make([]hybrid.Result, 0, len(results))
		for _, result := range results {
			if result.Score >= threshold {
				filtered = append(filtered, result)
			}
		}
		return filtered
	}

	filtered := filterAt(opts.ScoreThreshold)
	if len(filtered) > 0 {
		return filtered
	}

	if opts.ThresholdDegradeBy > 0 {
		degradedThreshold := opts.ScoreThreshold * opts.ThresholdDegradeBy
		if degradedThreshold < opts.ThresholdFloor {
			degradedThreshold = opts.ThresholdFloor
		}
		if degradedThreshold < opts.ScoreThreshold {
			filtered = filterAt(degradedThreshold)
			if len(filtered) > 0 {
				log.Printf("[Chat] RAG threshold degraded: query=%q original=%.2f degraded=%.2f kept=%d",
					queryText, opts.ScoreThreshold, degradedThreshold, len(filtered))
				return filtered
			}
		}
	}

	topScore := results[0].Score
	if opts.FallbackMinScore > 0 && topScore >= opts.FallbackMinScore {
		log.Printf("[Chat] RAG fallback top1: query=%q top_score=%.4f threshold=%.2f",
			queryText, topScore, opts.ScoreThreshold)
		return results[:1]
	}

	log.Printf("[Chat] RAG rejected: query=%q top_score=%.4f threshold=%.2f",
		queryText, topScore, opts.ScoreThreshold)
	return nil
}

func (o ragFilterOptions) withDefaults() ragFilterOptions {
	if o.TopK <= 0 {
		o.TopK = 5
	}
	if o.ScoreThreshold <= 0 {
		o.ScoreThreshold = 0.45
	}
	if o.FallbackMinScore < 0 {
		o.FallbackMinScore = 0
	}
	if o.ThresholdFloor < 0 {
		o.ThresholdFloor = 0
	}
	if o.ThresholdDegradeBy < 0 || o.ThresholdDegradeBy >= 1 {
		o.ThresholdDegradeBy = 0
	}
	return o
}
