package skill

import (
	"strings"
	"testing"

	"recallix/internal/repository"
)

func TestScheduleReturnsRelevantSkill(t *testing.T) {
	scheduler := NewScheduler()
	got := scheduler.Schedule("请帮我生成这段内容的参考文献和引用来源", []repository.Skill{
		{Name: "引用生成器", Description: "自动生成规范引用格式。当用户需要生成参考文献、引用来源、标注知识库内容出处、或要求提供引用信息时使用此技能。"},
		{Name: "文档总结器", Description: "总结长文档内容，提炼重点。"},
	})

	if len(got) == 0 {
		t.Fatal("expected at least one candidate")
	}
	if got[0].Skill.Name != "引用生成器" {
		t.Fatalf("top candidate = %q, want 引用生成器", got[0].Skill.Name)
	}
}

func TestScheduleReturnsNilForIrrelevantQuestion(t *testing.T) {
	scheduler := NewScheduler()
	got := scheduler.Schedule("今天天气怎么样", []repository.Skill{
		{Name: "引用生成器", Description: "自动生成规范引用格式。"},
	})
	if len(got) != 0 {
		t.Fatalf("got %d candidates, want 0", len(got))
	}
}

func TestBuildCandidateContextUsesMetadataOnly(t *testing.T) {
	got := BuildDecisionMessages("请生成引用", []Candidate{{
		Skill: repository.Skill{ID: "s1", Name: "引用生成器", Description: "自动生成规范引用格式。"},
	}})
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if !strings.Contains(got[1].Content, "引用生成器") || !strings.Contains(got[1].Content, "自动生成规范引用格式") {
		t.Fatalf("unexpected decision payload: %q", got[1].Content)
	}
}

func TestParseDecision(t *testing.T) {
	got, err := ParseDecision("```json\n{\"skill_ids\":[\"s1\",\"s2\"]}\n```")
	if err != nil {
		t.Fatalf("ParseDecision error: %v", err)
	}
	if len(got.SkillIDs) != 2 || got.SkillIDs[0] != "s1" || got.SkillIDs[1] != "s2" {
		t.Fatalf("unexpected decision: %+v", got)
	}
}

func TestExtractInstructionsRemovesFrontmatter(t *testing.T) {
	got := ExtractInstructions("---\nname: demo\ndescription: demo\n---\n\n# Demo\n\nbody")
	if got != "# Demo\n\nbody" {
		t.Fatalf("unexpected instructions: %q", got)
	}
}

func TestCleanRelativePathRejectsTraversal(t *testing.T) {
	if _, err := cleanRelativePath("../secret.txt"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestToolDefinitionsHideExecuteWhenSandboxDisabled(t *testing.T) {
	tools := ToolDefinitions(false)
	for _, tool := range tools {
		if tool.Function.Name == ToolExecuteSkillScript {
			t.Fatal("execute_skill_script should not be exposed when sandbox is disabled")
		}
	}
}
