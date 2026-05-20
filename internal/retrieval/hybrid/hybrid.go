package hybrid

import (
	"context"
	"math"
	"sort"

	"gorm.io/gorm"

	"recallix/internal/model/llm"
	"recallix/internal/retrieval/keyword"
	"recallix/internal/retrieval/vectorstore"
)

type Result struct {
	ChunkID         string
	Content         string
	ContextHeader   string
	Seq             int
	Score           float64
	ParentChunkID   string
	KnowledgeID     string
	KnowledgeBaseID string
}

type Service struct {
	db    *gorm.DB
	embed *llm.EmbeddingClient
	vs    vectorstore.VectorStore
}

func NewService(db *gorm.DB, embed *llm.EmbeddingClient, vs vectorstore.VectorStore) *Service {
	return &Service{db: db, embed: embed, vs: vs}
}

func (s *Service) Search(userID, kbID string, query string, topK int) ([]Result, error) {
	queryEmb, err := s.embed.Embed(query)
	if err != nil {
		return nil, err
	}
	return s.SearchWithEmbedding(userID, kbID, query, queryEmb, topK)
}

func (s *Service) SearchWithEmbedding(userID, kbID string, query string, queryEmb []float32, topK int) ([]Result, error) {
	vecResults, err := s.vs.SearchChunks(context.Background(), userID, kbID, queryEmb, topK*2)
	if err != nil {
		vecResults = make([]vectorstore.SearchResult, 0)
	}

	kwHits, err := keyword.Search(s.db, userID, kbID, query, topK*2)
	if err != nil {
		kwHits = nil
	}
	kwScores := make(map[string]float64)
	maxKW := 0.0
	for _, h := range kwHits {
		kwScores[h.ID] = h.Score
		if h.Score > maxKW {
			maxKW = h.Score
		}
	}
	for id := range kwScores {
		if maxKW > 0 {
			kwScores[id] = kwScores[id] / maxKW
		}
	}

	maxVec := float32(0)
	for _, r := range vecResults {
		if r.Score > maxVec {
			maxVec = r.Score
		}
	}

	type candidate struct {
		id    string
		score float64
	}
	candidatesMap := make(map[string]float64)
	for _, r := range vecResults {
		vecScore := 0.0
		if maxVec > 0 {
			vecScore = float64(r.Score / maxVec)
		}
		candidatesMap[r.ID] = 0.7*vecScore + 0.3*kwScores[r.ID]
	}
	for id, kwScore := range kwScores {
		if _, ok := candidatesMap[id]; !ok {
			candidatesMap[id] = 0.3 * kwScore
		}
	}

	candidates := make([]candidate, 0, len(candidatesMap))
	for id, score := range candidatesMap {
		candidates = append(candidates, candidate{id, score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}

	candidateIDs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		candidateIDs = append(candidateIDs, c.id)
	}
	var chunks []struct {
		ID              string
		Content         string
		ContextHeader   string
		Seq             int
		ParentChunkID   *string
		KnowledgeID     string
		KnowledgeBaseID string
	}
	if len(candidateIDs) > 0 {
		_ = s.db.Table("chunks").Where("id IN ?", candidateIDs).Find(&chunks).Error
	}
	chunkMap := make(map[string]struct {
		Content         string
		ContextHeader   string
		Seq             int
		ParentChunkID   string
		KnowledgeID     string
		KnowledgeBaseID string
	})
	for _, c := range chunks {
		pid := ""
		if c.ParentChunkID != nil {
			pid = *c.ParentChunkID
		}
		chunkMap[c.ID] = struct {
			Content         string
			ContextHeader   string
			Seq             int
			ParentChunkID   string
			KnowledgeID     string
			KnowledgeBaseID string
		}{c.Content, c.ContextHeader, c.Seq, pid, c.KnowledgeID, c.KnowledgeBaseID}
	}

	results := make([]Result, 0, len(candidates))
	for _, c := range candidates {
		info, ok := chunkMap[c.id]
		if !ok {
			continue
		}
		results = append(results, Result{
			ChunkID:         c.id,
			Content:         info.Content,
			ContextHeader:   info.ContextHeader,
			Seq:             info.Seq,
			Score:           math.Round(c.score*10000) / 10000,
			ParentChunkID:   info.ParentChunkID,
			KnowledgeID:     info.KnowledgeID,
			KnowledgeBaseID: info.KnowledgeBaseID,
		})
	}
	return results, nil
}
