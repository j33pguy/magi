package db

import (
	"os"
	"testing"
)

func TestHybridFetchKDefaultsToDeeperOverfetch(t *testing.T) {
	t.Setenv("MAGI_HYBRID_FETCH_MULTIPLIER", "")
	if got := hybridFetchK(5); got != 25 {
		t.Fatalf("hybridFetchK(5) = %d, want 25", got)
	}
}

func TestHybridFetchKHonorsMinimumCandidatePool(t *testing.T) {
	t.Setenv("MAGI_HYBRID_FETCH_MULTIPLIER", "")
	if got := hybridFetchK(1); got != 20 {
		t.Fatalf("hybridFetchK(1) = %d, want 20", got)
	}
}

func TestHybridFetchKUsesEnvOverride(t *testing.T) {
	t.Setenv("MAGI_HYBRID_FETCH_MULTIPLIER", "8")
	if got := hybridFetchK(5); got != 40 {
		t.Fatalf("hybridFetchK(5) with env override = %d, want 40", got)
	}
}

func TestHybridFetchKIgnoresInvalidEnvOverride(t *testing.T) {
	original := os.Getenv("MAGI_HYBRID_FETCH_MULTIPLIER")
	t.Setenv("MAGI_HYBRID_FETCH_MULTIPLIER", "garbage")
	if got := hybridFetchK(5); got != 25 {
		t.Fatalf("hybridFetchK(5) with invalid env override = %d, want 25", got)
	}
	_ = original
}
