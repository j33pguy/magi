package contradiction

import (
	"testing"
)

func TestScoreContradiction_NumericChange(t *testing.T) {
	tests := []struct {
		name        string
		newText     string
		existing    string
		wantMin     float64
		wantReason  string
	}{
		{
			name:       "port number change detected",
			newText:    "the API server is on port 9090",
			existing:   "the API server is on port 8080",
			wantMin:    0.7,
			wantReason: "numeric value differs",
		},
		{
			name:       "port number change",
			newText:    "port 8301 is grpc",
			existing:   "port 8300 is grpc",
			wantMin:    0.7,
			wantReason: "numeric value differs",
		},
		{
			name:       "version change",
			newText:    "running version 3.2",
			existing:   "running version 2.1",
			wantMin:    0.7,
			wantReason: "numeric value differs",
		},
		{
			name:     "same numbers no contradiction",
			newText:  "the API server is on port 8080",
			existing: "the API server is on port 8080",
			wantMin:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, reason := scoreContradiction(tt.newText, tt.existing)
			if score < tt.wantMin {
				t.Errorf("scoreContradiction() score = %v, want >= %v", score, tt.wantMin)
			}
			if tt.wantReason != "" && !contains(reason, tt.wantReason) {
				t.Errorf("scoreContradiction() reason = %q, want to contain %q", reason, tt.wantReason)
			}
		})
	}
}

func TestScoreContradiction_BooleanFlip(t *testing.T) {
	tests := []struct {
		name     string
		newText  string
		existing string
		wantMin  float64
	}{
		{
			name:     "enabled vs disabled",
			newText:  "the firewall is disabled",
			existing: "the firewall is enabled",
			wantMin:  0.6,
		},
		{
			name:     "true vs false",
			newText:  "auto-deploy is false",
			existing: "auto-deploy is true",
			wantMin:  0.6,
		},
		{
			name:     "active vs inactive",
			newText:  "the service is inactive",
			existing: "the service is active",
			wantMin:  0.6,
		},
		{
			name:     "same state no contradiction",
			newText:  "the firewall is enabled",
			existing: "the firewall is enabled",
			wantMin:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _ := scoreContradiction(tt.newText, tt.existing)
			if score < tt.wantMin {
				t.Errorf("scoreContradiction() score = %v, want >= %v", score, tt.wantMin)
			}
		})
	}
}

func TestScoreContradiction_ReplacementLanguage(t *testing.T) {
	tests := []struct {
		name     string
		newText  string
		existing string
		wantMin  float64
	}{
		{
			name:     "changed to",
			newText:  "changed to port 9090",
			existing: "running on port 8080",
			wantMin:  0.4,
		},
		{
			name:     "now uses",
			newText:  "now uses postgres instead",
			existing: "uses mysql for storage",
			wantMin:  0.4,
		},
		{
			name:     "no longer",
			newText:  "no longer using redis",
			existing: "uses redis for caching",
			wantMin:  0.4,
		},
		{
			name:     "was X now Y",
			newText:  "was on port 8080, now on port 9090",
			existing: "the server is on port 8080",
			wantMin:  0.4,
		},
		{
			name:     "no replacement language",
			newText:  "compute-node is running well",
			existing: "compute-node has good uptime",
			wantMin:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _ := scoreContradiction(tt.newText, tt.existing)
			if score < tt.wantMin {
				t.Errorf("scoreContradiction() score = %v, want >= %v", score, tt.wantMin)
			}
		})
	}
}

func TestScoreContradiction_CombinedHeuristics(t *testing.T) {
	// Multiple heuristics should stack up to 1.0
	score, reason := scoreContradiction(
		"changed to port 9090, service is now disabled",
		"running on port 8080, service is enabled",
	)
	if score < 0.7 {
		t.Errorf("combined heuristics score = %v, want >= 0.7", score)
	}
	if score > 1.0 {
		t.Errorf("combined heuristics score = %v, want <= 1.0", score)
	}
	if reason == "" {
		t.Error("combined heuristics should have a reason")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 120); got != "short" {
		t.Errorf("truncate(short) = %q, want %q", got, "short")
	}

	long := "this is a very long string that should be truncated because it exceeds the maximum allowed length for display purposes in the contradiction candidate summary"
	got := truncate(long, 50)
	if len(got) > 50 {
		t.Errorf("truncate() len = %d, want <= 50", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("truncate() should end with ..., got %q", got)
	}
}

func TestExtractKeywordNumbers(t *testing.T) {
	pairs := extractKeywordNumbers("server is on port 9090 and version 3.2")
	if nums, ok := pairs["port"]; !ok || len(nums) == 0 || nums[0] != "9090" {
		t.Errorf("expected port=9090, got %v", pairs["port"])
	}
	if nums, ok := pairs["version"]; !ok || len(nums) == 0 || nums[0] != "3.2" {
		t.Errorf("expected version=3.2, got %v", pairs["version"])
	}
}

func TestContainsWord(t *testing.T) {
	if !containsWord("the service is enabled", "enabled") {
		t.Error("expected to find 'enabled'")
	}
	if containsWord("the service is disabled", "enabled") {
		t.Error("should not match 'enabled' in 'disabled'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
