package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"recallix/internal/document/chunker"
	"recallix/internal/document/parser"
	"recallix/internal/model/llm"
	"recallix/internal/repository"
	"recallix/internal/retrieval/keyword"
	"recallix/internal/retrieval/vectorstore"
	"recallix/internal/shared"
	"recallix/internal/storage"
	"recallix/internal/task"
)

type DocProcessor struct {
	db    *gorm.DB
	embed *llm.EmbeddingClient
	vs    *vectorstore.MilvusStore
	store *storage.FileStorage
}

func NewDocProcessor(db *gorm.DB, embed *llm.EmbeddingClient, vs *vectorstore.MilvusStore, store *storage.FileStorage) *DocProcessor {
	return &DocProcessor{db: db, embed: embed, vs: vs, store: store}
}

func (dp *DocProcessor) HandleDocumentProcess(t *asynq.Task) error {
	var payload task.DocumentProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	var knowledge repository.Knowledge
	if err := dp.db.Where("id = ?", payload.KnowledgeID).First(&knowledge).Error; err != nil {
		return fmt.Errorf("knowledge not found: %w", err)
	}

	if knowledge.ParseStatus == "done" {
		log.Printf("[DocProcessor] Knowledge %s already processed, skipping", knowledge.ID)
		return nil
	}

	knowledge.ParseStatus = "processing"
	dp.db.Save(&knowledge)

	ext := filepath.Ext(knowledge.FileName)
	if _, err := parser.GetParser(ext); err != nil {
		knowledge.ParseStatus = "failed"
		dp.db.Save(&knowledge)
		return fmt.Errorf("unsupported format: %w, %w", err, asynq.SkipRetry)
	}

	reader, err := dp.store.Open(context.Background(), knowledge.FilePath)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", knowledge.FilePath, err)
	}
	fileContent, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return fmt.Errorf("read source file %s: %w", knowledge.FilePath, err)
	}
	mdContent, err := parser.ParseBytes(ext, fileContent)
	if err != nil {
		return fmt.Errorf("parse file %s: %w", knowledge.FilePath, err)
	}

	cfg := chunker.DefaultConfig()
	cfg.Strategy = "auto"

	results := chunker.Chunk(mdContent.Text, cfg)
	if len(results) == 0 {
		knowledge.ParseStatus = "failed"
		dp.db.Save(&knowledge)
		return fmt.Errorf("chunking produced no results: %w", asynq.SkipRetry)
	}

	log.Printf("[DocProcessor] Knowledge %s: parsing done, %d chunks generated", knowledge.ID, len(results))

	total := len(results)
	hasChunkFailure := false
	for i, r := range results {
		ctxHeader := r.ContextHeader
		content := r.Content
		if ctxHeader != "" {
			content = ctxHeader + "\n\n" + content
		}

		chunk := repository.Chunk{
			ID:            shared.NewID(),
			UserID:        knowledge.UserID,
			KnowledgeID:   knowledge.ID,
			Content:       r.Content,
			Seq:           r.Seq,
			StartPos:      r.StartPos,
			EndPos:        r.EndPos,
			ContextHeader: r.ContextHeader,
			ChunkType:     "text",
			CreatedAt:     time.Now(),
		}

		if err := dp.db.Create(&chunk).Error; err != nil {
			log.Printf("[DocProcessor] Failed to save chunk %d: %v", i, err)
			hasChunkFailure = true
			continue
		}
		if err := keyword.IndexChunk(dp.db, chunk, knowledge.KnowledgeBaseID); err != nil {
			log.Printf("[DocProcessor] Failed to index keywords for chunk %d: %v", i, err)
			hasChunkFailure = true
			continue
		}

		emb, err := dp.embed.Embed(content)
		if err != nil {
			log.Printf("[DocProcessor] Failed to embed chunk %d: %v", i, err)
			hasChunkFailure = true
			continue
		}

		var kb repository.KnowledgeBase
		if err := dp.db.Where("id = ?", knowledge.KnowledgeBaseID).First(&kb).Error; err == nil {
			_ = kb
		}

		if dp.vs != nil {
			if err := dp.vs.InsertChunk(vectorstore.VectorRecord{
				ChunkID:         chunk.ID,
				UserID:          knowledge.UserID,
				KnowledgeBaseID: knowledge.KnowledgeBaseID,
				Embedding:       emb,
			}); err != nil {
				log.Printf("[DocProcessor] Failed to insert vector for chunk %d: %v", i, err)
				hasChunkFailure = true
				continue
			}
		} else {
			log.Printf("[DocProcessor] Skipped vector insertion for chunk %d (Milvus unavailable)", i)
			hasChunkFailure = true
		}

		if (i+1)%10 == 0 || i == total-1 {
			log.Printf("[DocProcessor] Progress: %d/%d chunks processed", i+1, total)
		}
	}

	if hasChunkFailure {
		knowledge.ParseStatus = "failed"
		dp.db.Save(&knowledge)
		log.Printf("[DocProcessor] Knowledge %s: processing failed because one or more chunks were incomplete", knowledge.ID)
		return nil
	}

	knowledge.ParseStatus = "done"
	dp.db.Save(&knowledge)

	log.Printf("[DocProcessor] Knowledge %s: processing completed", knowledge.ID)
	return nil
}
