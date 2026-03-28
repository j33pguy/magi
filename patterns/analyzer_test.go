package patterns

import (
	"fmt"
	"testing"
	"time"

	"github.com/j33pguy/claude-memory/db"
)

func TestAnalyze_EmptyInput(t *testing.T) {
	a := &Analyzer{}
	result := a.Analyze(nil)
	if result != nil {
		t.Errorf("expected nil, got %d patterns", len(result))
	}

	result = a.Analyze([]*db.Memory{})
	if result != nil {
		t.Errorf("expected nil for empty slice, got %d patterns", len(result))
	}
}

func TestAnalyze_TechPreference_Frequency(t *testing.T) {
	memories := make([]*db.Memory, 5)
	for i := range memories {
		memories[i] = &db.Memory{
			ID:      fmt.Sprintf("mem-%d", i),
			Content: fmt.Sprintf("Built a new gRPC service for the %d API", i),
			Speaker: "j33p",
		}
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternPreference && containsString(p.Description, "gRPC") {
			found = true
			if len(p.Evidence) < 3 {
				t.Errorf("expected 3+ evidence IDs, got %d", len(p.Evidence))
			}
			if p.Confidence <= 0 || p.Confidence > 1 {
				t.Errorf("confidence out of range: %f", p.Confidence)
			}
		}
	}
	if !found {
		t.Error("expected gRPC preference pattern, not found")
	}
}

func TestAnalyze_TechPreference_ExplicitPrefer(t *testing.T) {
	memories := []*db.Memory{
		{ID: "1", Content: "I always use Go for backend services, written in Go", Speaker: "j33p"},
		{ID: "2", Content: "I prefer to use Go for CLI tools, switched to golang", Speaker: "j33p"},
		{ID: "3", Content: "Built the new service in Go", Speaker: "j33p"},
		{ID: "4", Content: "Go module setup for the proxy", Speaker: "j33p"},
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternPreference && containsString(p.Description, "Go") && containsString(p.Description, "Prefers") {
			found = true
		}
	}
	if !found {
		t.Error("expected explicit Go preference pattern, not found")
	}
}

func TestAnalyze_TechPreference_Avoids(t *testing.T) {
	memories := []*db.Memory{
		{ID: "1", Content: "Don't use Python for this, it's too slow", Speaker: "j33p"},
		{ID: "2", Content: "Avoid Python for performance-critical paths", Speaker: "j33p"},
		{ID: "3", Content: "Never use Python in the data pipeline", Speaker: "j33p"},
		{ID: "4", Content: "Python script for one-off migration", Speaker: "j33p"},
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternPreference && containsString(p.Description, "Python") && containsString(p.Description, "Avoids") {
			found = true
		}
	}
	if !found {
		t.Error("expected Python avoidance pattern, not found")
	}
}

func TestAnalyze_BelowThreshold_NoPattern(t *testing.T) {
	memories := []*db.Memory{
		{ID: "1", Content: "Used Docker once", Speaker: "j33p"},
		{ID: "2", Content: "Something unrelated", Speaker: "j33p"},
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	for _, p := range patterns {
		if containsString(p.Description, "Docker") {
			t.Error("should not detect pattern with fewer than 3 mentions")
		}
	}
}

func TestAnalyze_WorkTimingPattern_Weekends(t *testing.T) {
	// Create memories: homelab on weekends, work on weekdays
	var memories []*db.Memory

	// Saturday memories (homelab)
	sat := time.Date(2025, 3, 15, 14, 0, 0, 0, time.UTC) // Saturday
	for i := 0; i < 5; i++ {
		memories = append(memories, &db.Memory{
			ID:        fmt.Sprintf("sat-%d", i),
			Content:   fmt.Sprintf("Proxmox VM setup task %d", i),
			Speaker:   "j33p",
			Area:      "homelab",
			CreatedAt: sat.Add(time.Duration(i) * time.Hour).Format(time.DateTime),
		})
	}

	// Sunday memories (homelab)
	sun := time.Date(2025, 3, 16, 10, 0, 0, 0, time.UTC) // Sunday
	for i := 0; i < 3; i++ {
		memories = append(memories, &db.Memory{
			ID:        fmt.Sprintf("sun-%d", i),
			Content:   fmt.Sprintf("Network config task %d", i),
			Speaker:   "j33p",
			Area:      "homelab",
			CreatedAt: sun.Add(time.Duration(i) * time.Hour).Format(time.DateTime),
		})
	}

	// Weekday memories (work)
	mon := time.Date(2025, 3, 17, 9, 0, 0, 0, time.UTC) // Monday
	for i := 0; i < 3; i++ {
		memories = append(memories, &db.Memory{
			ID:        fmt.Sprintf("mon-%d", i),
			Content:   fmt.Sprintf("Work task %d", i),
			Speaker:   "j33p",
			Area:      "work",
			CreatedAt: mon.Add(time.Duration(i) * time.Hour).Format(time.DateTime),
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternWorkPattern && containsString(p.Description, "homelab") && containsString(p.Description, "weekend") {
			found = true
			if p.Area != "homelab" {
				t.Errorf("expected area=homelab, got %s", p.Area)
			}
		}
	}
	if !found {
		t.Error("expected weekend homelab work pattern, not found")
	}
}

func TestAnalyze_CommsStyle_Concise(t *testing.T) {
	var memories []*db.Memory
	for i := 0; i < 20; i++ {
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("m-%d", i),
			Content: fmt.Sprintf("short note %d", i),
			Speaker: "j33p",
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternCommsStyle && containsString(p.Description, "Concise") {
			found = true
		}
	}
	if !found {
		t.Error("expected concise communicator pattern, not found")
	}
}

func TestAnalyze_DecisionPattern_Security(t *testing.T) {
	memories := []*db.Memory{
		{ID: "1", Content: "Checked the auth token flow before we decided on the API design", Speaker: "j33p"},
		{ID: "2", Content: "Going with mTLS after reviewing security implications", Speaker: "j33p"},
		{ID: "3", Content: "Settled on RBAC after evaluating permission models and security", Speaker: "j33p"},
		{ID: "4", Content: "Something unrelated about lunch", Speaker: "j33p"},
		{ID: "5", Content: "Regular code review", Speaker: "j33p"},
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternDecisionStyle && containsString(p.Description, "security") {
			found = true
		}
	}
	if !found {
		t.Error("expected security-conscious decision pattern, not found")
	}
}

func TestClampConfidence(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{-1.0, 0.1},
		{0.0, 0.1},
		{0.05, 0.1},
		{0.5, 0.5},
		{1.0, 0.95},
		{2.0, 0.95},
	}
	for _, tt := range tests {
		got := clampConfidence(tt.in)
		if got != tt.want {
			t.Errorf("clampConfidence(%f) = %f, want %f", tt.in, got, tt.want)
		}
	}
}

func TestUniqueIDs(t *testing.T) {
	ids := []string{"a", "b", "a", "c", "b"}
	got := uniqueIDs(ids)
	if len(got) != 3 {
		t.Errorf("expected 3 unique IDs, got %d", len(got))
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
