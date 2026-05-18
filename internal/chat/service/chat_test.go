package service

import (
	"testing"

	"recallix/internal/retrieval/hybrid"
)

var testRAGFilterOptions = ragFilterOptions{
	TopK:               5,
	ScoreThreshold:     0.45,
	FallbackMinScore:   0,
	ThresholdFloor:     0,
	ThresholdDegradeBy: 0,
}

func TestFilterRAGResultsKeepsAboveThreshold(t *testing.T) {
	results := []hybrid.Result{{Score: 0.70}, {Score: 0.50}, {Score: 0.20}}
	filtered := filterRAGResults("q", results, testRAGFilterOptions)
	if len(filtered) != 2 {
		t.Fatalf("got %d results, want 2", len(filtered))
	}
}

func TestFilterRAGResultsRejectsBelowThresholdWithoutDegrade(t *testing.T) {
	results := []hybrid.Result{{Score: 0.40}, {Score: 0.34}, {Score: 0.10}}
	filtered := filterRAGResults("q", results, testRAGFilterOptions)
	if len(filtered) != 0 {
		t.Fatalf("got %d results, want 0", len(filtered))
	}
}

func TestFilterRAGResultsDoesNotFallbackToTop1ByDefault(t *testing.T) {
	results := []hybrid.Result{{Score: 0.20}, {Score: 0.10}}
	filtered := filterRAGResults("q", results, testRAGFilterOptions)
	if len(filtered) != 0 {
		t.Fatalf("got %d results, want 0", len(filtered))
	}
}

func TestFilterRAGResultsRejectsVeryLowScores(t *testing.T) {
	results := []hybrid.Result{{Score: 0.14}, {Score: 0.10}}
	filtered := filterRAGResults("q", results, testRAGFilterOptions)
	if len(filtered) != 0 {
		t.Fatalf("got %d results, want 0", len(filtered))
	}
}

func TestFilterRAGResultsCanDegradeWhenConfigured(t *testing.T) {
	results := []hybrid.Result{{Score: 0.40}, {Score: 0.34}, {Score: 0.10}}
	filtered := filterRAGResults("q", results, ragFilterOptions{
		TopK:               5,
		ScoreThreshold:     0.45,
		ThresholdFloor:     0.30,
		ThresholdDegradeBy: 0.70,
	})
	if len(filtered) != 2 {
		t.Fatalf("got %d results, want 2", len(filtered))
	}
}

func TestFilterRAGResultsCanFallbackWhenConfigured(t *testing.T) {
	results := []hybrid.Result{{Score: 0.20}, {Score: 0.10}}
	filtered := filterRAGResults("q", results, ragFilterOptions{
		TopK:             5,
		ScoreThreshold:   0.45,
		FallbackMinScore: 0.15,
	})
	if len(filtered) != 1 || filtered[0].Score != 0.20 {
		t.Fatalf("unexpected filtered results: %+v", filtered)
	}
}
