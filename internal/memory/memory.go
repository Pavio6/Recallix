package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"recallix/internal/model/llm"
	"recallix/internal/repository"
	"recallix/internal/retrieval/vectorstore"
	"recallix/internal/shared"
)

type Service struct {
	db    *gorm.DB
	chat  *llm.ChatClient
	embed *llm.EmbeddingClient
	vs    *vectorstore.MilvusStore
}

type Candidate struct {
	Action     string `json:"action"`
	MemoryType string `json:"memory_type"`
	MemoryText string `json:"memory_text"`
}

type Decision struct {
	Action         string `json:"action"`
	TargetMemoryID string `json:"target_memory_id"`
	MemoryType     string `json:"memory_type"`
	MemoryText     string `json:"memory_text"`
}

func NewService(db *gorm.DB, chat *llm.ChatClient, embed *llm.EmbeddingClient, vs *vectorstore.MilvusStore) *Service {
	return &Service{db: db, chat: chat, embed: embed, vs: vs}
}

func (s *Service) ProcessExtraction(userID, question, answer string) error {
	candidate, err := s.ExtractCandidate(question, answer)
	if err != nil {
		return err
	}
	if candidate.Action == "skip" || candidate.MemoryText == "" {
		return nil
	}

	emb, err := s.embed.Embed(candidate.MemoryText)
	if err != nil {
		return fmt.Errorf("embed candidate memory: %w", err)
	}

	similar, err := s.RecallWithEmbedding(userID, emb, 3)
	if err != nil {
		return fmt.Errorf("recall similar memories: %w", err)
	}

	decision := Decision{
		Action:     "create",
		MemoryType: candidate.MemoryType,
		MemoryText: candidate.MemoryText,
	}
	if len(similar) > 0 {
		decision, err = s.Decide(candidate, similar)
		if err != nil {
			return err
		}
	}

	switch decision.Action {
	case "skip":
		return nil
	case "update":
		finalEmb, err := s.embeddingForDecision(decision, candidate, emb)
		if err != nil {
			return err
		}
		return s.updateMemory(userID, decision, finalEmb)
	case "create":
		finalEmb, err := s.embeddingForDecision(decision, candidate, emb)
		if err != nil {
			return err
		}
		return s.createMemory(userID, decision, finalEmb)
	default:
		return fmt.Errorf("unknown memory decision action: %s", decision.Action)
	}
}

func (s *Service) embeddingForDecision(decision Decision, candidate Candidate, candidateEmbedding []float32) ([]float32, error) {
	if decision.MemoryText == candidate.MemoryText {
		return candidateEmbedding, nil
	}
	emb, err := s.embed.Embed(decision.MemoryText)
	if err != nil {
		return nil, fmt.Errorf("embed final memory: %w", err)
	}
	return emb, nil
}

func (s *Service) ExtractCandidate(question, answer string) (Candidate, error) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: `You are a memory extraction system.
Analyze the conversation and decide whether there is any stable long-term user information worth remembering.
Return ONLY JSON with this schema:
{"action":"create|skip","memory_type":"preference|profile|project|fact","memory_text":"string"}

Rules:
- Use "skip" if there is nothing worth remembering long-term.
- Extract one concise fact in first person when possible.
- Use "preference" for stable likes/dislikes or working style.
- Use "profile" for user identity/background.
- Use "project" for ongoing project details.
- Use "fact" for other useful long-term facts.`},
		{Role: "user", Content: fmt.Sprintf("User: %s\nAssistant: %s", question, answer)},
	}

	raw, err := s.chat.Chat(messages)
	if err != nil {
		return Candidate{}, fmt.Errorf("extract candidate memory: %w", err)
	}
	var candidate Candidate
	if err := parseJSON(raw, &candidate); err != nil {
		return Candidate{}, fmt.Errorf("parse candidate memory: %w", err)
	}
	normalizeCandidate(&candidate)
	return candidate, nil
}

func (s *Service) Decide(candidate Candidate, similar []repository.Memory) (Decision, error) {
	var sb strings.Builder
	for _, mem := range similar {
		sb.WriteString(fmt.Sprintf("- id=%s type=%s text=%s\n", mem.ID, mem.MemoryType, mem.MemoryText))
	}
	messages := []llm.ChatMessage{
		{Role: "system", Content: `You manage long-term user memories.
Given one candidate memory and similar existing memories, decide whether to:
- "create": candidate is meaningfully new.
- "update": candidate should replace or refine one existing memory.
- "skip": candidate is duplicate or not useful.

Return ONLY JSON with this schema:
{"action":"create|update|skip","target_memory_id":"string","memory_type":"preference|profile|project|fact","memory_text":"string"}

Rules:
- For "update", target_memory_id must be one of the existing memory ids.
- For "create", target_memory_id must be empty.
- Prefer "skip" for near-duplicates.
- Prefer "update" when the new candidate supersedes or materially refines an existing memory.`},
		{Role: "user", Content: fmt.Sprintf("Candidate:\n%s\n\nExisting memories:\n%s", mustJSON(candidate), sb.String())},
	}

	raw, err := s.chat.Chat(messages)
	if err != nil {
		return Decision{}, fmt.Errorf("decide memory action: %w", err)
	}
	var decision Decision
	if err := parseJSON(raw, &decision); err != nil {
		return Decision{}, fmt.Errorf("parse memory decision: %w", err)
	}
	normalizeDecision(&decision)
	return decision, nil
}

func (s *Service) createMemory(userID string, decision Decision, emb []float32) error {
	memory := repository.Memory{
		ID:         shared.NewID(),
		UserID:     userID,
		MemoryText: decision.MemoryText,
		MemoryType: decision.MemoryType,
		Importance: 1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.db.Create(&memory).Error; err != nil {
		return fmt.Errorf("create memory: %w", err)
	}
	if s.vs != nil {
		if err := s.vs.InsertMemory(vectorstore.VectorRecord{
			MemoryID:  memory.ID,
			UserID:    userID,
			Embedding: emb,
		}); err != nil {
			return fmt.Errorf("insert memory vector: %w", err)
		}
	}
	return nil
}

func (s *Service) updateMemory(userID string, decision Decision, emb []float32) error {
	if decision.TargetMemoryID == "" {
		return fmt.Errorf("update memory missing target_memory_id")
	}
	var memory repository.Memory
	if err := s.db.Where("id = ? AND user_id = ?", decision.TargetMemoryID, userID).First(&memory).Error; err != nil {
		return fmt.Errorf("find memory to update: %w", err)
	}
	memory.MemoryText = decision.MemoryText
	memory.MemoryType = decision.MemoryType
	memory.UpdatedAt = time.Now()
	if err := s.db.Save(&memory).Error; err != nil {
		return fmt.Errorf("update memory: %w", err)
	}
	if s.vs != nil {
		if err := s.vs.DeleteMemory(memory.ID); err != nil {
			return fmt.Errorf("delete old memory vector: %w", err)
		}
		if err := s.vs.InsertMemory(vectorstore.VectorRecord{
			MemoryID:  memory.ID,
			UserID:    userID,
			Embedding: emb,
		}); err != nil {
			return fmt.Errorf("insert updated memory vector: %w", err)
		}
	}
	return nil
}

func (s *Service) Recall(userID, query string, topK int) ([]repository.Memory, error) {
	queryEmb, err := s.embed.Embed(query)
	if err != nil {
		return nil, err
	}
	return s.RecallWithEmbedding(userID, queryEmb, topK)
}

func (s *Service) RecallWithEmbedding(userID string, queryEmb []float32, topK int) ([]repository.Memory, error) {
	if s.vs == nil {
		return []repository.Memory{}, nil
	}
	results, err := s.vs.SearchMemories(userID, queryEmb, topK)
	if err != nil || len(results) == 0 {
		return []repository.Memory{}, nil
	}
	var memories []repository.Memory
	for _, r := range results {
		var mem repository.Memory
		if err := s.db.Where("id = ? AND user_id = ?", r.ID, userID).First(&mem).Error; err != nil {
			continue
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

func (s *Service) List(userID string) ([]repository.Memory, error) {
	var memories []repository.Memory
	if err := s.db.Where("user_id = ?", userID).Order("created_at desc").Find(&memories).Error; err != nil {
		return nil, err
	}
	if memories == nil {
		memories = []repository.Memory{}
	}
	return memories, nil
}

func (s *Service) Delete(userID, memoryID string) error {
	if err := s.db.Where("id = ? AND user_id = ?", memoryID, userID).Delete(&repository.Memory{}).Error; err != nil {
		return err
	}
	if s.vs != nil {
		return s.vs.DeleteMemory(memoryID)
	}
	return nil
}

func parseJSON(raw string, out any) error {
	content := strings.TrimSpace(raw)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	return json.Unmarshal([]byte(content), out)
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func normalizeCandidate(candidate *Candidate) {
	candidate.Action = normalizeAction(candidate.Action, "create")
	candidate.MemoryType = normalizeMemoryType(candidate.MemoryType)
	candidate.MemoryText = strings.TrimSpace(candidate.MemoryText)
}

func normalizeDecision(decision *Decision) {
	decision.Action = normalizeAction(decision.Action, "create")
	decision.MemoryType = normalizeMemoryType(decision.MemoryType)
	decision.MemoryText = strings.TrimSpace(decision.MemoryText)
	decision.TargetMemoryID = strings.TrimSpace(decision.TargetMemoryID)
}

func normalizeAction(action, fallback string) string {
	switch strings.TrimSpace(action) {
	case "create", "update", "skip":
		return strings.TrimSpace(action)
	default:
		return fallback
	}
}

func normalizeMemoryType(memoryType string) string {
	switch strings.TrimSpace(memoryType) {
	case "preference", "profile", "project", "fact":
		return strings.TrimSpace(memoryType)
	default:
		return "fact"
	}
}
