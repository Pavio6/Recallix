package vectorstore

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// PGStore implements VectorStore backed by PostgreSQL pgvector.
type PGStore struct {
	db  *gorm.DB
	dim int
}

func NewPGStore(db *gorm.DB, dim int) (*PGStore, error) {
	if dim <= 0 {
		dim = 1024
	}

	s := &PGStore{db: db, dim: dim}

	if err := s.initTables(); err != nil {
		return nil, fmt.Errorf("pgvector init tables: %w", err)
	}

	return s, nil
}

func (s *PGStore) initTables() error {
	// Enable extension (idempotent)
	if err := s.db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return fmt.Errorf("enable vector extension: %w", err)
	}

	// Create chunk_embeddings table
	if err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS chunk_embeddings (
			chunk_id          VARCHAR(64) PRIMARY KEY,
			user_id           VARCHAR(64) NOT NULL,
			knowledge_base_id VARCHAR(64) NOT NULL DEFAULT '',
			embedding         vector(1024) NOT NULL
		)
	`).Error; err != nil {
		return fmt.Errorf("create chunk_embeddings: %w", err)
	}

	// Create memory_embeddings table
	if err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memory_embeddings (
			memory_id VARCHAR(64) PRIMARY KEY,
			user_id   VARCHAR(64) NOT NULL,
			embedding vector(1024) NOT NULL
		)
	`).Error; err != nil {
		return fmt.Errorf("create memory_embeddings: %w", err)
	}

	// Create HNSW indexes (idempotent via IF NOT EXISTS)
	if err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_chunk_emb_l2
		ON chunk_embeddings
		USING hnsw (embedding vector_l2_ops)
	`).Error; err != nil {
		log.Printf("[PGStore] chunk HNSW index warning: %v", err)
	}

	if err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_chunk_emb_user
		ON chunk_embeddings (user_id)
	`).Error; err != nil {
		log.Printf("[PGStore] chunk user index warning: %v", err)
	}

	if err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memory_emb_l2
		ON memory_embeddings
		USING hnsw (embedding vector_l2_ops)
	`).Error; err != nil {
		log.Printf("[PGStore] memory HNSW index warning: %v", err)
	}

	return nil
}

// InsertChunk inserts or updates a chunk embedding.
func (s *PGStore) InsertChunk(ctx context.Context, rec VectorRecord) error {
	vec := formatVector(rec.Embedding)
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO chunk_embeddings (chunk_id, user_id, knowledge_base_id, embedding)
		VALUES (?, ?, ?, ?::vector)
		ON CONFLICT (chunk_id) DO UPDATE SET embedding = EXCLUDED.embedding
	`, rec.ChunkID, rec.UserID, rec.KnowledgeBaseID, vec).Error
}

// InsertMemory inserts or updates a memory embedding.
func (s *PGStore) InsertMemory(ctx context.Context, rec VectorRecord) error {
	vec := formatVector(rec.Embedding)
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO memory_embeddings (memory_id, user_id, embedding)
		VALUES (?, ?, ?::vector)
		ON CONFLICT (memory_id) DO UPDATE SET embedding = EXCLUDED.embedding
	`, rec.MemoryID, rec.UserID, vec).Error
}

// SearchChunks performs L2-based vector search over chunk embeddings.
func (s *PGStore) SearchChunks(ctx context.Context, userID, kbID string, embedding []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	vec := formatVector(embedding)

	query := `
		SELECT chunk_id, knowledge_base_id, 1 - (embedding <=> ?::vector) AS score
		FROM chunk_embeddings
		WHERE user_id = ?
	`
	args := []interface{}{vec, userID}

	if kbID != "" {
		query += ` AND knowledge_base_id = ?`
		args = append(args, kbID)
	}

	query += ` ORDER BY embedding <=> ?::vector LIMIT ?`
	args = append(args, vec, topK)

	rows, err := s.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0, topK)
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.KnowledgeBaseID, &r.Score); err != nil {
			return nil, fmt.Errorf("scan chunk result: %w", err)
		}
		results = append(results, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows iteration: %w", rows.Err())
	}
	return results, nil
}

// SearchMemories performs L2-based vector search over memory embeddings.
func (s *PGStore) SearchMemories(ctx context.Context, userID string, embedding []float32, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}
	vec := formatVector(embedding)

	rows, err := s.db.WithContext(ctx).Raw(`
		SELECT memory_id, 1 - (embedding <=> ?::vector) AS score
		FROM memory_embeddings
		WHERE user_id = ?
		ORDER BY embedding <=> ?::vector LIMIT ?
	`, vec, userID, vec, topK).Rows()
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0, topK)
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.Score); err != nil {
			return nil, fmt.Errorf("scan memory result: %w", err)
		}
		results = append(results, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows iteration: %w", rows.Err())
	}
	return results, nil
}

// DeleteChunk removes a chunk embedding.
func (s *PGStore) DeleteChunk(ctx context.Context, chunkID string) error {
	return s.db.WithContext(ctx).Exec(`DELETE FROM chunk_embeddings WHERE chunk_id = ?`, chunkID).Error
}

// DeleteMemory removes a memory embedding.
func (s *PGStore) DeleteMemory(ctx context.Context, memoryID string) error {
	return s.db.WithContext(ctx).Exec(`DELETE FROM memory_embeddings WHERE memory_id = ?`, memoryID).Error
}

// DeleteByKnowledgeID removes all chunk embeddings for a specific knowledge document.
func (s *PGStore) DeleteByKnowledgeID(ctx context.Context, userID, kbID, knowledgeID string) error {
	return s.db.WithContext(ctx).Exec(`
		DELETE FROM chunk_embeddings 
		WHERE user_id = ? AND knowledge_base_id = ? AND chunk_id IN (
			SELECT id FROM chunks WHERE knowledge_id = ?
		)
	`, userID, kbID, knowledgeID).Error
}

// Close is a no-op for pgvector (connection managed by GORM).
func (s *PGStore) Close() error {
	return nil
}

// formatVector converts []float32 to pgvector literal format: [1.2,3.4,...]
func formatVector(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
