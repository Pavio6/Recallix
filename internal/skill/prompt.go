package skill

import (
	"encoding/json"
	"fmt"
	"strings"

	"recallix/internal/model/llm"
)

type Metadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Decision struct {
	SkillIDs []string `json:"skill_ids"`
}

type Selection struct {
	Candidates []Candidate
	Selected   []Candidate
	RawOutput  string
}

func BuildDecisionMessages(question string, candidates []Candidate) []llm.ChatMessage {
	metadata := make([]Metadata, 0, len(candidates))
	for _, candidate := range candidates {
		metadata = append(metadata, Metadata{
			ID:          candidate.Skill.ID,
			Name:        candidate.Skill.Name,
			Description: candidate.Skill.Description,
		})
	}
	metadataJSON, _ := json.Marshal(metadata)

	return []llm.ChatMessage{
		{Role: "system", Content: `You decide which candidate skills must be loaded before answering the user's request.
Return ONLY JSON in this exact shape:
{"skill_ids":["..."]}
Rules:
- Use only candidate skill ids from the provided list.
- Select a skill only if its instructions are likely to materially improve the final answer.
- If no skill is truly needed, return {"skill_ids":[]}.
- Prefer the smallest sufficient set; do not select redundant skills.`},
		{Role: "user", Content: fmt.Sprintf("Question:\n%s\n\nCandidate skills:\n%s", question, string(metadataJSON))},
	}
}

func ParseDecision(raw string) (Decision, error) {
	content := strings.TrimSpace(raw)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var decision Decision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return Decision{}, err
	}
	if decision.SkillIDs == nil {
		decision.SkillIDs = []string{}
	}
	return decision, nil
}

func FilterSelected(candidates []Candidate, selectedIDs []string) []Candidate {
	if len(candidates) == 0 || len(selectedIDs) == 0 {
		return nil
	}
	allowed := make(map[string]Candidate, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.Skill.ID] = candidate
	}
	selected := make([]Candidate, 0, len(selectedIDs))
	seen := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		candidate, ok := allowed[id]
		if !ok {
			continue
		}
		selected = append(selected, candidate)
		seen[id] = struct{}{}
	}
	return selected
}
