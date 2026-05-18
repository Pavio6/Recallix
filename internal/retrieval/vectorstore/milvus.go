package vectorstore

import (
	"context"
	"fmt"
	"log"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"

	"recallix/internal/config"
)

type MilvusStore struct {
	cfg *config.Config
	cli client.Client
	dim int
}

func New(cfg *config.Config) (*MilvusStore, error) {
	c, err := client.NewClient(context.Background(), client.Config{
		Address: fmt.Sprintf("%s:%s", cfg.MilvusHost, cfg.MilvusPort),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create milvus client: %w", err)
	}

	dim := cfg.EmbeddingDimension
	if dim <= 0 {
		dim = 1024
	}

	store := &MilvusStore{cfg: cfg, cli: c, dim: dim}

	store.initCollection("chunk_collection", "chunk_id")
	store.initCollection("memory_collection", "memory_id")

	return store, nil
}

func (m *MilvusStore) initCollection(name, pkField string) {
	ctx := context.Background()

	has, err := m.cli.HasCollection(ctx, name)
	if err != nil {
		log.Printf("[Milvus] HasCollection(%s) error: %v", name, err)
		return
	}
	if has {
		return
	}

	schema := entity.NewSchema().WithName(name).WithDescription("Recallix " + name).
		WithField(entity.NewField().WithName(pkField).WithDataType(entity.FieldTypeVarChar).WithMaxLength(64).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName("user_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName("embedding").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(m.dim))).
		WithField(entity.NewField().WithName("knowledge_base_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(64))

	if err := m.cli.CreateCollection(ctx, schema, 1); err != nil {
		log.Printf("[Milvus] CreateCollection(%s) error: %v", name, err)
		return
	}

	idx, err := entity.NewIndexHNSW(entity.COSINE, 16, 256)
	if err != nil {
		log.Printf("[Milvus] NewIndex(%s) error: %v", name, err)
		return
	}

	if err := m.cli.CreateIndex(ctx, name, "embedding", idx, false); err != nil {
		log.Printf("[Milvus] CreateIndex(%s) error: %v", name, err)
	}
}

type VectorRecord struct {
	ChunkID         string
	MemoryID        string
	UserID          string
	KnowledgeBaseID string
	Embedding       []float32
}

func (m *MilvusStore) InsertChunk(rec VectorRecord) error {
	ctx := context.Background()
	chunkCol := entity.NewColumnVarChar("chunk_id", []string{rec.ChunkID})
	userCol := entity.NewColumnVarChar("user_id", []string{rec.UserID})
	kbCol := entity.NewColumnVarChar("knowledge_base_id", []string{rec.KnowledgeBaseID})
	embCol := entity.NewColumnFloatVector("embedding", m.dim, [][]float32{rec.Embedding})
	_, err := m.cli.Insert(ctx, "chunk_collection", "", chunkCol, userCol, kbCol, embCol)
	return err
}

func (m *MilvusStore) InsertMemory(rec VectorRecord) error {
	ctx := context.Background()
	memCol := entity.NewColumnVarChar("memory_id", []string{rec.MemoryID})
	userCol := entity.NewColumnVarChar("user_id", []string{rec.UserID})
	kbCol := entity.NewColumnVarChar("knowledge_base_id", []string{""})
	embCol := entity.NewColumnFloatVector("embedding", m.dim, [][]float32{rec.Embedding})
	_, err := m.cli.Insert(ctx, "memory_collection", "", memCol, userCol, kbCol, embCol)
	return err
}

type SearchResult struct {
	ID              string
	Score           float32
	KnowledgeBaseID string
}

func (m *MilvusStore) SearchChunks(userID, kbID string, embedding []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	ctx := context.Background()
	if err := m.cli.LoadCollection(ctx, "chunk_collection", false); err != nil {
		return nil, err
	}

	vec := entity.FloatVector(embedding)
	sp, _ := entity.NewIndexHNSWSearchParam(64)

	expr := fmt.Sprintf(`user_id == "%s"`, userID)
	if kbID != "" {
		expr += fmt.Sprintf(` && knowledge_base_id == "%s"`, kbID)
	}

	searchResult, err := m.cli.Search(ctx, "chunk_collection", nil, expr,
		[]string{"chunk_id", "knowledge_base_id"},
		[]entity.Vector{vec}, "embedding",
		entity.COSINE, topK, sp)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0)
	for _, sr := range searchResult {
		for i := 0; i < sr.ResultCount; i++ {
			idCol, ok := sr.IDs.(*entity.ColumnVarChar)
			if !ok {
				continue
			}
			id, err := idCol.ValueByIdx(i)
			if err != nil {
				continue
			}
			results = append(results, SearchResult{
				ID:    id,
				Score: sr.Scores[i],
			})
		}
	}
	return results, nil
}

func (m *MilvusStore) SearchMemories(userID string, embedding []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}
	ctx := context.Background()
	if err := m.cli.LoadCollection(ctx, "memory_collection", false); err != nil {
		return nil, err
	}

	vec := entity.FloatVector(embedding)
	sp, _ := entity.NewIndexHNSWSearchParam(64)
	expr := fmt.Sprintf(`user_id == "%s"`, userID)

	searchResult, err := m.cli.Search(ctx, "memory_collection", nil, expr,
		[]string{"memory_id"},
		[]entity.Vector{vec}, "embedding",
		entity.COSINE, topK, sp)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0)
	for _, sr := range searchResult {
		for i := 0; i < sr.ResultCount; i++ {
			idCol, ok := sr.IDs.(*entity.ColumnVarChar)
			if !ok {
				continue
			}
			id, err := idCol.ValueByIdx(i)
			if err != nil {
				continue
			}
			results = append(results, SearchResult{
				ID:    id,
				Score: sr.Scores[i],
			})
		}
	}
	return results, nil
}

func (m *MilvusStore) DeleteChunk(chunkID string) error {
	ctx := context.Background()
	expr := fmt.Sprintf(`chunk_id == "%s"`, chunkID)
	return m.cli.Delete(ctx, "chunk_collection", "", expr)
}

func (m *MilvusStore) DeleteMemory(memoryID string) error {
	ctx := context.Background()
	expr := fmt.Sprintf(`memory_id == "%s"`, memoryID)
	return m.cli.Delete(ctx, "memory_collection", "", expr)
}

func (m *MilvusStore) Close() error {
	if m.cli != nil {
		return m.cli.Close()
	}
	return nil
}
