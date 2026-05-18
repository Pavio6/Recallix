package memory

import "testing"

func TestNormalizeCandidateFallsBackToFact(t *testing.T) {
	candidate := Candidate{Action: "weird", MemoryType: "unknown", MemoryText: "  hello  "}
	normalizeCandidate(&candidate)
	if candidate.Action != "create" {
		t.Fatalf("action = %q, want create", candidate.Action)
	}
	if candidate.MemoryType != "fact" {
		t.Fatalf("memory type = %q, want fact", candidate.MemoryType)
	}
	if candidate.MemoryText != "hello" {
		t.Fatalf("memory text = %q, want hello", candidate.MemoryText)
	}
}

func TestParseJSONToleratesExtraText(t *testing.T) {
	var candidate Candidate
	if err := parseJSON("```json\n{\"action\":\"skip\",\"memory_type\":\"fact\",\"memory_text\":\"\"}\n```", &candidate); err != nil {
		t.Fatalf("parseJSON error: %v", err)
	}
	if candidate.Action != "skip" {
		t.Fatalf("action = %q, want skip", candidate.Action)
	}
}
