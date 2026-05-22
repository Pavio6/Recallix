package prompt

import (
	"fmt"
	"strings"

	"recallix/internal/repository"
)

func BuildSystemPrompt() string {
	return `You are Recallix, an intelligent AI assistant with access to a knowledge base. 
Follow these rules:
1. Answer based on the provided knowledge context when available.
2. If you don't know the answer, say so honestly - don't make things up.
3. Use proper markdown formatting: tables must have each row on its own line with correct |---| separators.
4. Keep answers concise but thorough.
5. Consider the user's long-term memory if provided.`
}

func BuildNoRetrievalSystemPrompt(intent string) string {
	switch intent {
	case "greeting":
		return `You are Recallix, a warm and concise AI assistant.
The user is greeting you, thanking you, or saying farewell. Reply naturally and briefly.
Do not mention the knowledge base or retrieval unless the user asks about it.`
	case "chitchat":
		return `You are Recallix, a helpful and natural AI assistant.
Answer conversationally. Do not mention the knowledge base or retrieval unless the user asks about it.`
	case "follow_up":
		return `You are Recallix, a helpful AI assistant.
Answer using the existing conversation history. Do not claim to have searched the knowledge base unless retrieved context is provided.`
	case "summarize":
		return `You are Recallix, a helpful AI assistant.
The user wants a summary of the existing conversation. Use the dialogue history only and do not claim to have searched the knowledge base.`
	case "clarification":
		return `You are Recallix, a helpful AI assistant.
The user's question is ambiguous or incomplete. Your job is to ask a clarifying question to help them specify what they're looking for.

Rules:
1. Identify what's ambiguous: the topic, the scope, the context, or the specific aspect they want to know about.
2. Ask ONE clear, concise clarifying question. Do not ask multiple questions at once.
3. Offer 2-4 plausible interpretations to help the user choose, phrased as options.
4. Be warm and helpful, not robotic.
5. Do NOT attempt to answer the question yet - wait for the user's clarification first.
6. Do NOT mention the knowledge base or retrieval.

Example:
User: "进程的优缺点是什么？"
Assistant: "您指的是哪方面的进程呢？
- 操作系统中的进程（Process）与线程的对比？
- 某个具体业务流程的优缺点？
- 其他含义？

请告诉我更多细节，我可以帮您更准确地查找。"`
	default:
		return `You are Recallix, a helpful AI assistant.
Answer naturally and honestly. Do not claim to have searched the knowledge base unless retrieved context is provided.`
	}
}

func BuildContext(chunks []repository.Chunk) string {
	if len(chunks) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【Knowledge Base Context】\n")
	for i, chunk := range chunks {
		if chunk.ContextHeader != "" {
			sb.WriteString(fmt.Sprintf("[Source %d] %s\n%s\n", i+1, chunk.ContextHeader, chunk.Content))
			continue
		}
		sb.WriteString(fmt.Sprintf("[Source %d] %s\n", i+1, chunk.Content))
	}
	return sb.String()
}

func BuildMemoryContext(memories []repository.Memory) string {
	if len(memories) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【User Memory】\n")
	for _, mem := range memories {
		sb.WriteString(fmt.Sprintf("- %s\n", mem.MemoryText))
	}
	return sb.String()
}
