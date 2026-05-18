package prompt

import (
	"strings"
	"testing"

	"recallix/internal/repository"
)

func TestBuildContextIncludesContextHeader(t *testing.T) {
	got := BuildContext([]repository.Chunk{{
		ContextHeader: "# 一级标题 > ## 二级标题",
		Content:       "正文内容",
	}})
	if !strings.Contains(got, "# 一级标题 > ## 二级标题\n正文内容") {
		t.Fatalf("context missing header: %q", got)
	}
}

func TestBuildContextWithoutContextHeader(t *testing.T) {
	got := BuildContext([]repository.Chunk{{Content: "正文内容"}})
	if !strings.Contains(got, "[Source 1] 正文内容") {
		t.Fatalf("unexpected context: %q", got)
	}
}
