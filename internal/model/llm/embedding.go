package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"recallix/internal/config"
)

type EmbeddingClient struct {
	cfg    *config.Config
	client *http.Client
}

func NewEmbeddingClient(cfg *config.Config) *EmbeddingClient {
	return &EmbeddingClient{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type embeddingReq struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (e *EmbeddingClient) Embed(text string) ([]float32, error) {
	req := embeddingReq{Model: e.cfg.EmbeddingModel, Input: text}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", e.embedURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.cfg.EmbeddingModelAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.cfg.EmbeddingModelAPIKey)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API error: %s", resp.Status)
	}

	var embResp embeddingResp
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, err
	}
	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data")
	}
	return embResp.Data[0].Embedding, nil
}

func (e *EmbeddingClient) EmbedBatch(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := e.Embed(text)
		if err != nil {
			return nil, fmt.Errorf("embedding batch error at %d: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}

func (e *EmbeddingClient) embedURL() string {
	base := e.cfg.EmbeddingModelBaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if !strings.HasSuffix(base, "/embeddings") {
		base = strings.TrimRight(base, "/") + "/embeddings"
	}
	return base
}
