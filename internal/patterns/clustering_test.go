package patterns

import (
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func TestDetectTopicBursts(t *testing.T) {
	now := time.Now().UTC()
	var memories []*db.Memory

	// Recent burst (last 7 days)
	for i := 0; i < 6; i++ {
		memories = append(memories, &db.Memory{
			ID:        "r" + string(rune('a'+i)),
			Tags:      []string{"topic:magi"},
			CreatedAt: now.AddDate(0, 0, -1-i).Format(time.DateTime),
		})
	}

	// Baseline activity (8-35 days ago)
	for i := 0; i < 4; i++ {
		memories = append(memories, &db.Memory{
			ID:        "b" + string(rune('a'+i)),
			Tags:      []string{"topic:magi"},
			CreatedAt: now.AddDate(0, 0, -10-(i*5)).Format(time.DateTime),
		})
	}

	analyzer := &Analyzer{}
	patterns := analyzer.detectTopicBursts(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternTopicBurst {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected topic burst pattern")
	}
}
