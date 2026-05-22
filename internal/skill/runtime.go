package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"recallix/internal/model/llm"
	"recallix/internal/repository"
	"recallix/internal/sandbox"
	"recallix/internal/storage"
)

const (
	ToolReadSkill          = "read_skill"
	ToolReadSkillFile      = "read_skill_file"
	ToolExecuteSkillScript = "execute_skill_script"
)

type Runtime struct {
	store   *storage.FileStorage
	sandbox *sandbox.Executor
	allowed map[string]repository.Skill
}

type ToolResult struct {
	Output        string
	LoadedSkillID string
}

func NewRuntime(store *storage.FileStorage, executor *sandbox.Executor, candidates []Candidate) *Runtime {
	allowed := make(map[string]repository.Skill, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.Skill.ID] = candidate.Skill
	}
	return &Runtime{store: store, sandbox: executor, allowed: allowed}
}

func ToolDefinitions(sandboxEnabled bool) []llm.Tool {
	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        ToolReadSkill,
				Description: "Load the full instructions for a candidate skill when its metadata matches the user's task.",
				Parameters: objectSchema(map[string]any{
					"skill_id": map[string]any{"type": "string", "description": "Candidate skill id to read"},
				}, []string{"skill_id"}),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        ToolReadSkillFile,
				Description: "Read an additional file from a loaded skill directory, such as references or examples.",
				Parameters: objectSchema(map[string]any{
					"skill_id":  map[string]any{"type": "string"},
					"file_path": map[string]any{"type": "string", "description": "Relative file path inside the skill directory"},
				}, []string{"skill_id", "file_path"}),
			},
		},
	}
	if sandboxEnabled {
		tools = append(tools, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        ToolExecuteSkillScript,
				Description: "Execute a script bundled with a skill in the configured sandbox.",
				Parameters: objectSchema(map[string]any{
					"skill_id":    map[string]any{"type": "string"},
					"script_path": map[string]any{"type": "string"},
					"args":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"input":       map[string]any{"type": "string"},
				}, []string{"skill_id", "script_path"}),
			},
		})
	}
	return tools
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func (r *Runtime) Execute(ctx context.Context, call llm.ToolCall) ToolResult {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: invalid arguments: %v", err)}
	}
	switch call.Function.Name {
	case ToolReadSkill:
		return r.readSkill(ctx, asString(args["skill_id"]))
	case ToolReadSkillFile:
		return r.readSkillFile(ctx, asString(args["skill_id"]), asString(args["file_path"]))
	case ToolExecuteSkillScript:
		return r.executeSkillScript(ctx, asString(args["skill_id"]), asString(args["script_path"]), asStrings(args["args"]), asString(args["input"]))
	default:
		return ToolResult{Output: "tool error: unknown tool"}
	}
}

func (r *Runtime) readSkill(ctx context.Context, skillID string) ToolResult {
	skill, ok := r.allowed[skillID]
	if !ok {
		return ToolResult{Output: "tool error: skill is not available"}
	}
	content, err := r.readObject(ctx, skill.EntryFileURI)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	files, _ := r.listRelativeFiles(ctx, skill)
	var sb strings.Builder
	sb.WriteString("=== Skill: ")
	sb.WriteString(skill.Name)
	sb.WriteString(" ===\n")
	if skill.Description != "" {
		sb.WriteString("Description: ")
		sb.WriteString(skill.Description)
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Instructions\n")
	sb.WriteString(ExtractInstructions(content))
	if len(files) > 1 {
		sb.WriteString("\n\n## Available Files\n")
		for _, file := range files {
			if file != "SKILL.md" {
				sb.WriteString("- ")
				sb.WriteString(file)
				sb.WriteString("\n")
			}
		}
	}
	return ToolResult{Output: sb.String(), LoadedSkillID: skill.ID}
}

func (r *Runtime) readSkillFile(ctx context.Context, skillID, relativePath string) ToolResult {
	skill, ok := r.allowed[skillID]
	if !ok {
		return ToolResult{Output: "tool error: skill is not available"}
	}
	uri, err := r.fileURI(skill, relativePath)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	content, err := r.readObject(ctx, uri)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	return ToolResult{Output: content, LoadedSkillID: skill.ID}
}

func (r *Runtime) executeSkillScript(ctx context.Context, skillID, scriptPath string, args []string, input string) ToolResult {
	skill, ok := r.allowed[skillID]
	if !ok {
		return ToolResult{Output: "tool error: skill is not available"}
	}
	if r.sandbox == nil || !r.sandbox.Enabled() {
		return ToolResult{Output: "tool error: skill sandbox is disabled"}
	}
	tempDir, err := os.MkdirTemp("", "recallix-skill-*")
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	defer os.RemoveAll(tempDir)
	if err := r.materialize(ctx, skill, tempDir); err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	clean, err := cleanRelativePath(scriptPath)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	result, err := r.sandbox.Execute(ctx, filepath.Join(tempDir, clean), tempDir, args, input)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("tool error: %v", err)}
	}
	return ToolResult{Output: fmt.Sprintf("exit_code=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr), LoadedSkillID: skill.ID}
}

func (r *Runtime) readObject(ctx context.Context, uri string) (string, error) {
	reader, err := r.store.Open(ctx, uri)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (r *Runtime) fileURI(skill repository.Skill, relativePath string) (string, error) {
	clean, err := cleanRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	return r.store.URIForObject(path.Join(skill.StoragePrefix, clean))
}

func (r *Runtime) listRelativeFiles(ctx context.Context, skill repository.Skill) ([]string, error) {
	keys, err := r.store.ListPrefix(ctx, skill.StoragePrefix)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(keys))
	prefix := strings.TrimSuffix(skill.StoragePrefix, "/") + "/"
	for _, key := range keys {
		files = append(files, strings.TrimPrefix(key, prefix))
	}
	return files, nil
}

func (r *Runtime) materialize(ctx context.Context, skill repository.Skill, target string) error {
	files, err := r.listRelativeFiles(ctx, skill)
	if err != nil {
		return err
	}
	for _, file := range files {
		uri, err := r.fileURI(skill, file)
		if err != nil {
			return err
		}
		content, err := r.readObject(ctx, uri)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func cleanRelativePath(relativePath string) (string, error) {
	clean := path.Clean(strings.TrimSpace(relativePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("invalid skill file path")
	}
	return clean, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asStrings(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func BuildCatalogContext(candidates []Candidate) string {
	if len(candidates) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【Available Skills】\n")
	sb.WriteString("You MUST consider the skills below. If a skill is relevant, call read_skill before using it. ")
	sb.WriteString("Do not assume instructions from the description alone.\n")
	for _, candidate := range candidates {
		sb.WriteString("- id=")
		sb.WriteString(candidate.Skill.ID)
		sb.WriteString(" name=")
		sb.WriteString(candidate.Skill.Name)
		sb.WriteString(": ")
		sb.WriteString(candidate.Skill.Description)
		sb.WriteString("\n")
	}
	return sb.String()
}
