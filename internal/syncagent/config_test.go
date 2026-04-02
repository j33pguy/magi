package syncagent

import "testing"

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
		want    bool
	}{
		{"**/projects/**/*.jsonl", "foo/projects/a/b.jsonl", true},
		{"**/CLAUDE.md", "repo/CLAUDE.md", true},
		{"**/*.md", "repo/notes.txt", false},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.pattern, tt.target); got != tt.want {
			t.Fatalf("matchGlob(%q,%q)=%v want %v", tt.pattern, tt.target, got, tt.want)
		}
	}
}

func TestApplyPrivacy(t *testing.T) {
	got := applyPrivacy("api_key=secretsecret123", PrivacyConfig{RedactSecrets: true})
	if got == "api_key=secretsecret123" {
		t.Fatal("expected redaction")
	}
}

func TestExtractContentNestedJSON(t *testing.T) {
	line := `{"message":{"content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}}`
	got := extractTurn(line).Content
	if got != "hello\nworld" {
		t.Fatalf("extractTurn().Content=%q want %q", got, "hello\nworld")
	}
}

func TestExtractTurnRoleAware(t *testing.T) {
	line := `{"role":"user","message":{"content":[{"type":"text","text":"please check the API migration"}]}}`
	turn := extractTurn(line)
	if turn.Role != "user" {
		t.Fatalf("turn.Role=%q want %q", turn.Role, "user")
	}
	if turn.Content != "please check the API migration" {
		t.Fatalf("turn.Content=%q", turn.Content)
	}
}

func TestFormatAndSummarizeTurns(t *testing.T) {
	turns := []claudeTurn{
		{Role: "user", Content: "Please investigate the failing endpoint"},
		{Role: "assistant", Content: "The issue is a missing auth header in v3"},
	}

	formatted := formatTurns(turns)
	wantFormatted := "[user] Please investigate the failing endpoint\n[assistant] The issue is a missing auth header in v3"
	if formatted != wantFormatted {
		t.Fatalf("formatted=%q want %q", formatted, wantFormatted)
	}

	summary := summarizeTurns(turns, 120)
	wantSummary := "user: Please investigate the failing endpoint -> assistant: The issue is a missing auth header in v3"
	if summary != wantSummary {
		t.Fatalf("summary=%q want %q", summary, wantSummary)
	}

	if got := summarySpeaker(turns); got != "assistant" {
		t.Fatalf("summarySpeaker=%q want %q", got, "assistant")
	}
}

func TestIdentityTags(t *testing.T) {
	cfg := &Config{Machine: MachineConfig{User: "UserA", ID: "MachineA"}}
	agent := AgentConfig{
		Type:         "claude",
		Name:         "claude-main",
		Visibility:   "team",
		Owner:        "UserA",
		Viewers:      []string{"UserB"},
		ViewerGroups: []string{"platform"},
	}
	tags := identityTags(cfg, agent, "conversation")
	want := []string{
		"user:UserA",
		"machine:MachineA",
		"identity:UserA.MachineA",
		"visibility:team",
		"owner:UserA",
		"viewer:UserB",
		"viewer_group:platform",
	}
	for _, w := range want {
		found := false
		for _, tag := range tags {
			if tag == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected tag %q in %v", w, tags)
		}
	}
}

func TestValidateVisibility(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{URL: "http://example.com"},
		Sync:   SyncConfig{Mode: "push"},
		Privacy: PrivacyConfig{
			Mode: "allowlist",
		},
		Agents: []AgentConfig{
			{
				Type:       "claude",
				Enabled:    true,
				Paths:      []string{"/tmp"},
				Visibility: "invalid",
			},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validate to reject invalid visibility")
	}
}

func TestValidateRejectsUnsupportedSyncMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{URL: "http://example.com"},
		Sync: SyncConfig{
			Mode: "pull",
		},
		Privacy: PrivacyConfig{
			Mode: "allowlist",
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validate to reject unsupported sync mode")
	}
}
