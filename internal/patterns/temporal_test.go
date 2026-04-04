package patterns

import (
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func TestApplyTemporalTrends_Emerging(t *testing.T) {
	now := time.Now().UTC()
	memories := []*db.Memory{
		{ID: "m1", CreatedAt: now.AddDate(0, 0, -5).Format(time.DateTime)},
		{ID: "m2", CreatedAt: now.AddDate(0, 0, -10).Format(time.DateTime)},
		{ID: "m3", CreatedAt: now.AddDate(0, 0, -15).Format(time.DateTime)},
	}
	patterns := []Pattern{{Type: PatternPreference, Description: "Prefers Go", Confidence: 0.6, Evidence: []string{"m1", "m2", "m3"}}}

	patterns = applyTemporalTrends(patterns, memories)

	if patterns[0].Trend != string(TrendEmerging) {
		t.Fatalf("expected emerging trend, got %q", patterns[0].Trend)
	}
	if patterns[0].FirstSeen == "" || patterns[0].LastSeen == "" {
		t.Fatalf("expected first/last seen to be set")
	}
}

func TestApplyTemporalTrends_Declining(t *testing.T) {
	now := time.Now().UTC()
	memories := []*db.Memory{
		{ID: "o1", CreatedAt: now.AddDate(0, 0, -90).Format(time.DateTime)},
		{ID: "o2", CreatedAt: now.AddDate(0, 0, -80).Format(time.DateTime)},
		{ID: "o3", CreatedAt: now.AddDate(0, 0, -70).Format(time.DateTime)},
		{ID: "o4", CreatedAt: now.AddDate(0, 0, -60).Format(time.DateTime)},
		{ID: "r1", CreatedAt: now.AddDate(0, 0, -5).Format(time.DateTime)},
	}
	patterns := []Pattern{{Type: PatternCommsStyle, Description: "Direct", Confidence: 0.6, Evidence: []string{"o1", "o2", "o3", "o4", "r1"}}}

	patterns = applyTemporalTrends(patterns, memories)

	if patterns[0].Trend != string(TrendDeclining) {
		t.Fatalf("expected declining trend, got %q", patterns[0].Trend)
	}
}

func TestApplySourceCorrelation(t *testing.T) {
	memories := []*db.Memory{
		{ID: "s1", Source: "discord", CreatedAt: time.Now().UTC().Format(time.DateTime)},
		{ID: "s2", Source: "webchat", CreatedAt: time.Now().UTC().Format(time.DateTime)},
	}
	patterns := []Pattern{{Type: PatternDecisionStyle, Description: "Security decisions", Confidence: 0.5, Evidence: []string{"s1", "s2"}}}

	patterns = applySourceCorrelation(patterns, memories)

	if len(patterns[0].Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(patterns[0].Sources))
	}
	if patterns[0].Confidence <= 0.5 {
		t.Fatalf("expected confidence to increase, got %.2f", patterns[0].Confidence)
	}
}
