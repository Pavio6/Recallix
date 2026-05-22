package chunker

import (
	"strings"
	"testing"
)

func TestHeadingPositionsUseRuneOffsets(t *testing.T) {
	text := "# 标题一\n\n中文正文\n\n## 标题二\n\n第二段内容"
	cfg := DefaultConfig()
	cfg.Strategy = "heading"

	results := Chunk(text, cfg)
	if len(results) != 2 {
		t.Fatalf("got %d chunks, want 2", len(results))
	}
	if results[0].StartPos != 0 || results[0].EndPos != 11 {
		t.Fatalf("first chunk positions = %d..%d, want 0..11", results[0].StartPos, results[0].EndPos)
	}
	if results[1].StartPos != 13 || results[1].EndPos != len([]rune(text)) {
		t.Fatalf("second chunk positions = %d..%d, want 13..%d", results[1].StartPos, results[1].EndPos, len([]rune(text)))
	}
}

func TestRecursivePositionsReferToOriginalText(t *testing.T) {
	text := "第一段内容。\n\n第二段内容。"
	cfg := DefaultConfig()
	cfg.Strategy = "recursive"
	cfg.ChunkSize = 6
	cfg.ChunkOverlap = 0

	results := Chunk(text, cfg)
	if len(results) != 2 {
		t.Fatalf("got %d chunks, want 2", len(results))
	}
	if results[0].StartPos != 0 || results[0].EndPos != 6 {
		t.Fatalf("first chunk positions = %d..%d, want 0..6", results[0].StartPos, results[0].EndPos)
	}
	if results[1].StartPos != 8 || results[1].EndPos != len([]rune(text)) {
		t.Fatalf("second chunk positions = %d..%d, want 8..%d", results[1].StartPos, results[1].EndPos, len([]rune(text)))
	}
}

func TestAutoUsesHeuristicForFormFeed(t *testing.T) {
	text := "第一页内容比较长，用来模拟 PDF 或 DOCX 转换后的第一页。\f第二页内容继续展开，用来验证分页符能触发启发式切分。"
	cfg := DefaultConfig()
	cfg.Strategy = "auto"
	cfg.ChunkSize = 30
	cfg.ChunkOverlap = 0

	results := Chunk(text, cfg)
	if len(results) != 2 {
		t.Fatalf("got %d chunks, want 2", len(results))
	}
	if results[0].Content != "第一页内容比较长，用来模拟 PDF 或 DOCX 转换后的第一页。" {
		t.Fatalf("first chunk = %q", results[0].Content)
	}
	if results[1].Content != "第二页内容继续展开，用来验证分页符能触发启发式切分。" {
		t.Fatalf("second chunk = %q", results[1].Content)
	}
}

func TestProfileCountsNumberedChineseHeadingWithoutSpace(t *testing.T) {
	profile := profileDocument("1、背景说明\n正文")
	if profile.numberedSectionCount != 1 {
		t.Fatalf("numberedSectionCount = %d, want 1", profile.numberedSectionCount)
	}
}

func TestHeuristicKeepsBoundaryHeadingWithFollowingBlock(t *testing.T) {
	text := "前言内容。" + strings.Repeat("补充说明。", 20) + "\n\nChapter 1. Overview\n章节内容。" + strings.Repeat("背景。", 8) + "\n\nChapter 2. Details\n更多内容"
	cfg := DefaultConfig()
	cfg.Strategy = "heuristic"
	cfg.ChunkSize = 80
	cfg.ChunkOverlap = 0

	results := Chunk(text, cfg)
	if len(results) < 2 {
		t.Fatalf("got %d chunks, want at least 2", len(results))
	}
	if results[1].Content == "" || results[1].Content[:7] != "Chapter" {
		t.Fatalf("second chunk should start with boundary heading, got %q", results[1].Content)
	}
}

func TestHeuristicIgnoresBoundariesInsideCodeFence(t *testing.T) {
	text := "说明\n```\nChapter 1. Not a heading\n```\n结尾"
	chapterOffset := len([]rune("说明\n```\n"))

	for _, boundary := range findHeuristicBoundaries(text) {
		if boundary == chapterOffset {
			t.Fatalf("code-fence chapter line was treated as heuristic boundary")
		}
	}
}
