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

type RerankClient struct {
	cfg    *config.Config
	client *http.Client
}

func NewRerankClient(cfg *config.Config) *RerankClient {
	return &RerankClient{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type RerankDoc struct {
	Text string `json:"text"`
}

// Standard format (Jina / Cohere / SiliconFlow / Voyage)
type stdRerankReq struct {
	Model     string      `json:"model"`
	Query     string      `json:"query"`
	Documents []RerankDoc `json:"documents"`
}

type stdRerankResp struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Aliyun DashScope format
type aliRerankInput struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type aliRerankReq struct {
	Model      string         `json:"model"`
	Input      aliRerankInput `json:"input"`
	Parameters struct{}       `json:"parameters"`
}

type aliRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type aliRerankOutput struct {
	Results []aliRerankResult `json:"results"`
}

type aliRerankResp struct {
	Output aliRerankOutput `json:"output"`
}

func (r *RerankClient) isAliyun() bool {
	return strings.Contains(r.cfg.RerankModelBaseURL, "dashscope")
}

func (r *RerankClient) Rerank(query string, docs []string) ([]float64, error) {
	if r.cfg.RerankModel == "" || len(docs) == 0 {
		scores := make([]float64, len(docs))
		for i := range scores {
			scores[i] = 1.0
		}
		return scores, nil
	}

	if r.isAliyun() {
		return r.rerankAliyun(query, docs)
	}
	return r.rerankStandard(query, docs)
}

func (r *RerankClient) rerankStandard(query string, docs []string) ([]float64, error) {
	rankDocs := make([]RerankDoc, len(docs))
	for i, d := range docs {
		rankDocs[i] = RerankDoc{Text: d}
	}
	req := stdRerankReq{Model: r.cfg.RerankModel, Query: query, Documents: rankDocs}
	body, _ := json.Marshal(req)

	respBody, err := r.doRequest(r.stdRerankURL(), body)
	if err != nil {
		return nil, err
	}

	var rankResp stdRerankResp
	if err := json.Unmarshal(respBody, &rankResp); err != nil {
		return nil, err
	}
	scores := make([]float64, len(docs))
	for _, result := range rankResp.Results {
		if result.Index < len(scores) {
			scores[result.Index] = result.RelevanceScore
		}
	}
	return scores, nil
}

func (r *RerankClient) rerankAliyun(query string, docs []string) ([]float64, error) {
	req := aliRerankReq{
		Model: r.cfg.RerankModel,
		Input: aliRerankInput{
			Query:     query,
			Documents: docs,
		},
	}
	body, _ := json.Marshal(req)

	respBody, err := r.doRequest(r.aliRerankURL(), body)
	if err != nil {
		return nil, err
	}

	var rankResp aliRerankResp
	if err := json.Unmarshal(respBody, &rankResp); err != nil {
		return nil, err
	}
	scores := make([]float64, len(docs))
	for _, result := range rankResp.Output.Results {
		if result.Index < len(scores) {
			scores[result.Index] = result.RelevanceScore
		}
	}
	return scores, nil
}

func (r *RerankClient) doRequest(url string, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if r.cfg.RerankModelAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+r.cfg.RerankModelAPIKey)
	}

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank API error: %s, body: %s", resp.Status, string(respBody))
	}
	return respBody, nil
}

func (r *RerankClient) stdRerankURL() string {
	base := r.cfg.RerankModelBaseURL
	if base == "" {
		base = "https://api.jina.ai/v1"
	}
	if !strings.HasSuffix(base, "/rerank") {
		base = strings.TrimRight(base, "/") + "/rerank"
	}
	return base
}

func (r *RerankClient) aliRerankURL() string {
	return "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"
}
