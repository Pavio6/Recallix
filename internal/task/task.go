package task

import (
	"encoding/json"

	"github.com/hibiken/asynq"

	"recallix/internal/shared"
)

const (
	TypeDocumentProcess = "document:process"
	TypeMemoryExtract   = "memory:extract"
)

type DocumentProcessPayload struct {
	KnowledgeID string `json:"knowledge_id"`
}

type MemoryExtractPayload struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	Question  string `json:"question"`
	Answer    string `json:"answer"`
}

func NewDocumentProcessTask(knowledgeID string) (*asynq.Task, error) {
	payload, _ := json.Marshal(DocumentProcessPayload{KnowledgeID: knowledgeID})
	return asynq.NewTask(TypeDocumentProcess, payload), nil
}

func NewMemoryExtractTask(userID, sessionID, question, answer string) (*asynq.Task, error) {
	payload, _ := json.Marshal(MemoryExtractPayload{
		UserID:    userID,
		SessionID: sessionID,
		Question:  question,
		Answer:    answer,
	})
	return asynq.NewTask(TypeMemoryExtract, payload), nil
}

func NewClient(redisAddr, redisPassword string, db int) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       db,
	})
}

func NewServer(redisAddr, redisPassword string, db int, concurrency int) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       db,
		},
		asynq.Config{Concurrency: concurrency},
	)
}

var _ = shared.NewID
