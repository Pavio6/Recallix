package vectorstore

import "context"

// VectorStore is the abstraction over vector storage backends (Milvus, pgvector, etc.).
type VectorStore interface {
	InsertChunk(ctx context.Context, rec VectorRecord) error
	InsertMemory(ctx context.Context, rec VectorRecord) error
	SearchChunks(ctx context.Context, userID, kbID string, embedding []float32, topK int) ([]SearchResult, error)
	SearchMemories(ctx context.Context, userID string, embedding []float32, topK int) ([]SearchResult, error)
	DeleteChunk(ctx context.Context, chunkID string) error
	DeleteMemory(ctx context.Context, memoryID string) error
	DeleteByKnowledgeID(ctx context.Context, userID, kbID, knowledgeID string) error
	Close() error
}

type VectorRecord struct {
	ChunkID         string
	MemoryID        string
	UserID          string
	KnowledgeBaseID string
	Embedding       []float32
}

type SearchResult struct {
	ID              string
	Score           float32
	KnowledgeBaseID string
}
