package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"recallix/internal/config"
)

type ChatClient struct {
	cfg    *config.Config
	client *http.Client
}

func NewChatClient(cfg *config.Config) *ChatClient {
	return &ChatClient{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type streamChunk struct {
	Choices []struct {
		Delta ChatMessage `json:"delta"`
	} `json:"choices"`
}

func (c *ChatClient) Chat(messages []ChatMessage) (string, error) {
	return c.ChatStream(messages, nil)
}

func (c *ChatClient) ChatStream(messages []ChatMessage, writer func(string) error) (string, error) {
	req := chatReq{
		Model:    c.cfg.ChatModel,
		Messages: messages,
		Stream:   true,
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", c.chatURL(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(httpReq)
	log.Printf("[Chat] POST %s (model=%s, msgs=%d)", c.chatURL(), c.cfg.ChatModel, len(messages))

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("chat API error: %s, body: %s", resp.Status, string(respBody))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			content := choice.Delta.Content
			if content != "" {
				fullContent.WriteString(content)
				if writer != nil {
					writer(content)
				}
			}
		}
	}

	return fullContent.String(), scanner.Err()
}

func (c *ChatClient) chatURL() string {
	base := c.cfg.ChatModelBaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if !strings.HasSuffix(base, "/chat/completions") {
		base = strings.TrimRight(base, "/") + "/chat/completions"
	}
	return base
}

func (c *ChatClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.ChatModelAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.ChatModelAPIKey)
	}
}
