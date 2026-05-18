package chunker

import "testing"

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
