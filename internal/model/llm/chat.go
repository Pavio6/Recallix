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
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type chatReq struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Tools      []Tool        `json:"tools,omitempty"`
	ToolChoice any           `json:"tool_choice,omitempty"`
	Stream     bool          `json:"stream"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatResponse struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
}

type streamChunk struct {
	Choices []struct {
		Delta ChatMessage `json:"delta"`
	} `json:"choices"`
}

type chatCompletionResp struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

func (c *ChatClient) Chat(messages []ChatMessage) (string, error) {
	return c.ChatStream(messages, nil)
}

func (c *ChatClient) ChatWithModel(model string, messages []ChatMessage) (string, error) {
	return c.ChatStreamWithModel(model, messages, nil)
}

func (c *ChatClient) ChatWithTools(model string, messages []ChatMessage, tools []Tool) (ChatResponse, error) {
	if model == "" {
		model = c.cfg.ChatModel
	}
	req := chatReq{Model: model, Messages: messages, Tools: tools, ToolChoice: "auto", Stream: false}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", c.chatURL(), bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	c.setHeaders(httpReq)
	log.Printf("[Chat] POST %s (model=%s, msgs=%d, tools=%d)", c.chatURL(), model, len(messages), len(tools))
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("chat API error: %s, body: %s", resp.Status, string(respBody))
	}
	var parsed chatCompletionResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, err
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, nil
	}
	message := parsed.Choices[0].Message
	return ChatResponse{
		Content:          message.Content,
		ReasoningContent: message.ReasoningContent,
		ToolCalls:        message.ToolCalls,
	}, nil
}

func (c *ChatClient) ChatStream(messages []ChatMessage, writer func(string) error) (string, error) {
	return c.ChatStreamWithModel(c.cfg.ChatModel, messages, writer)
}

func (c *ChatClient) ChatStreamWithModel(model string, messages []ChatMessage, writer func(string) error) (string, error) {
	if model == "" {
		model = c.cfg.ChatModel
	}
	req := chatReq{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", c.chatURL(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(httpReq)
	log.Printf("[Chat] POST %s (model=%s, msgs=%d)", c.chatURL(), model, len(messages))

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
