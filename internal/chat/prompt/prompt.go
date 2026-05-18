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
