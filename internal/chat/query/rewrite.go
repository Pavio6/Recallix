package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"recallix/internal/model/llm"
)

type Intent string

const (
	// IntentKBSearch 表示用户在询问需要依赖知识库回答的实质性问题，
	// 例如查找、解释、比较、列举或提取知识库中的内容。
	IntentKBSearch Intent = "kb_search"

	// IntentGreeting 表示纯问候、感谢或告别，没有实际的信息需求，
	// 例如“你好”“谢谢”“再见”。
	IntentGreeting Intent = "greeting"

	// IntentChitchat 表示普通闲聊，可以直接回答，不需要检索知识库，
	// 例如“你是谁”“讲个笑话”。
	IntentChitchat Intent = "chitchat"

	// IntentFollowUp 表示用户在追问前文内容，并且只依赖已有对话历史
	// 就可以回答，不需要再次检索知识库。
	IntentFollowUp Intent = "follow_up"

	// IntentSummarize 表示用户希望总结或回顾当前这段对话本身，
	// 而不是去知识库中检索文档内容。
	IntentSummarize Intent = "summarize"

	// IntentClarification 表示用户的问题存在歧义或信息不完整，
	// 但本质上仍属于知识查询路径，后续仍可能需要检索知识库。
	IntentClarification Intent = "clarification"
)

type UnderstandResult struct {
	RewriteQuery string `json:"rewrite_query"`
	Intent       Intent `json:"intent"`
}

// NeedsRetrieval reports whether the current turn should enter the KB retrieval
// path. The empty intent is treated as retrieval-needed for safety.
// Note: IntentClarification returns false because it should first ask the user
// to clarify the question before retrieving from the knowledge base.
func (r UnderstandResult) NeedsRetrieval() bool {
	switch r.Intent {
	case IntentGreeting, IntentChitchat, IntentFollowUp, IntentSummarize, IntentClarification:
		return false
	default:
		return true
	}
}

// Understand performs query rewriting and intent classification in one LLM call.
// On model or parsing failure it falls back to the safest behavior: use the
// original question and continue with KB retrieval.
func Understand(chat *llm.ChatClient, history []llm.ChatMessage, question string) UnderstandResult {
	fallback := UnderstandResult{
		RewriteQuery: question,
		Intent:       IntentKBSearch,
	}
	if chat == nil {
		return fallback
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: queryUnderstandingPrompt},
	}

	recentHistory := history
	if len(recentHistory) > 6 {
		recentHistory = recentHistory[len(recentHistory)-6:]
	}
	for _, msg := range recentHistory {
		if msg.Role == "user" || msg.Role == "assistant" {
			messages = append(messages, msg)
		}
	}
	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: fmt.Sprintf("Current user question:\n%s\n\nReturn JSON only.", question),
	})

	raw, err := chat.Chat(messages)
	if err != nil || strings.TrimSpace(raw) == "" {
		return fallback
	}

	result, ok := ParseUnderstandResult(raw)
	if !ok {
		return fallback
	}
	if strings.TrimSpace(result.RewriteQuery) == "" {
		result.RewriteQuery = question
	}
	if !isKnownIntent(result.Intent) {
		result.Intent = IntentKBSearch
	}
	return result
}

func ParseUnderstandResult(raw string) (UnderstandResult, bool) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return UnderstandResult{}, false
	}
	if result, ok := parseJSON(content); ok {
		return result, true
	}

	// Be tolerant of occasional markdown fences or extra prose from the model.
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return parseJSON(content[start : end+1])
	}
	return UnderstandResult{}, false
}

func parseJSON(content string) (UnderstandResult, bool) {
	var result UnderstandResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return UnderstandResult{}, false
	}
	result.RewriteQuery = strings.TrimSpace(result.RewriteQuery)
	result.Intent = Intent(strings.TrimSpace(string(result.Intent)))
	return result, true
}

func isKnownIntent(intent Intent) bool {
	switch intent {
	case IntentKBSearch, IntentGreeting, IntentChitchat, IntentFollowUp, IntentSummarize, IntentClarification:
		return true
	default:
		return false
	}
}

const queryUnderstandingPrompt = `You are a query understanding assistant for a knowledge-base chatbot.
You must perform exactly TWO tasks:
1. Rewrite the user's latest question into a concise, self-contained question, resolving references from conversation history when needed.
2. Classify the user's intent.

Choose exactly one intent:
- "greeting": pure greetings, thanks, or farewells with no substantive question.
- "kb_search": the user wants factual information that should be answered from the knowledge base, including searching, explaining, comparing, listing, or extracting stored knowledge.
- "clarification": the question is ambiguous or incomplete and likely needs knowledge-base retrieval to answer well.
- "follow_up": the question can be fully answered from prior dialogue alone and does not require new retrieval.
- "summarize": the user asks to summarize or review the conversation itself.
- "chitchat": casual conversation that needs no retrieval.

Decision rules:
- Default to "kb_search" when unsure.
- If the user asks a substantive factual question, do not classify it as "greeting" merely because it contains polite words.
- Use "follow_up" only when the answer can be produced from dialogue history alone without fetching new knowledge.
- Preserve concrete entities and core search terms in rewrite_query. Do not output meta-instructions such as "search the knowledge base for...".

Return ONLY one JSON object with this exact schema:
{"rewrite_query":"string","intent":"string"}

Examples:
Input: "早上好"
Output: {"rewrite_query":"早上好","intent":"greeting"}

Input: "线程和进程有什么区别"
Output: {"rewrite_query":"线程和进程有什么区别","intent":"kb_search"}

Input: "它和线程有什么区别" (history discusses processes)
Output: {"rewrite_query":"进程和线程有什么区别","intent":"kb_search"}

Input: "把你刚才回答的第二点再展开"
Output: {"rewrite_query":"请展开说明上一条回答中的第二点","intent":"follow_up"}

Input: "总结一下我们刚才聊了什么"
Output: {"rewrite_query":"总结一下我们刚才聊了什么","intent":"summarize"}`
