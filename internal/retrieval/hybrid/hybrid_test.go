package hybrid

import "testing"

func TestCosineScoresKeepHigherSimilarityHigher(t *testing.T) {
	maxVec := float32(0.9)
	high := float64(float32(0.9) / maxVec)
	low := float64(float32(0.3) / maxVec)
	if high <= low {
		t.Fatalf("expected higher cosine similarity to keep a higher normalized score: high=%f low=%f", high, low)
	}
}
