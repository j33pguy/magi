package patterns

import (
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func TestDetectRelationshipPatterns(t *testing.T) {
	now := time.Now().UTC().Format(time.DateTime)
	memories := []*db.Memory{
		{ID: "m1", Content: "Alice deployed with Docker for the service", Area: "work", CreatedAt: now},
		{ID: "m2", Content: "Docker logs were reviewed by Alice", Area: "work", CreatedAt: now},
		{ID: "m3", Content: "Alice and Docker troubleshooting session", Area: "work", CreatedAt: now},
	}

	analyzer := &Analyzer{}
	patterns := analyzer.detectRelationshipPatterns(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternRelationship && p.Area == "work" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected relationship pattern")
	}
}
