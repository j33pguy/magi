package syncagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenClawSessionPayloads(t *testing.T) {
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "sessions", "test-session.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
		t.Fatal(err)
	}

	content := `{"type":"session","version":"1","id":"abc12345-session-id","timestamp":"2026-04-05T12:00:00Z","cwd":"/home/test"}
{"type":"model_change","id":"m1","parentId":"","timestamp":"2026-04-05T12:00:01Z","provider":"anthropic","modelId":"claude-3"}
{"type":"message","id":"msg1","parentId":"","timestamp":"2026-04-05T12:00:02Z","message":{"role":"user","content":"Hello, how are you?"}}
{"type":"message","id":"msg2","parentId":"msg1","timestamp":"2026-04-05T12:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"I'm doing well, thanks!"}]}}
{"type":"message","id":"msg3","parentId":"msg2","timestamp":"2026-04-05T12:00:04Z","message":{"role":"user","content":"What's the weather like?"}}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Machine: MachineConfig{ID: "test-machine"},
	}
	agent := AgentConfig{
		Type:       "openclaw",
		Name:       "test-openclaw",
		Enabled:    true,
		Paths:      []string{dir},
		Include:    []string{"**/*.jsonl"},
		Visibility: "internal",
	}
	privacy := PrivacyConfig{MaxFileSizeKB: 512}

	payloads, err := (openclawAdapter{}).Scan(cfg, agent, privacy)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(payloads) == 0 {
		t.Fatal("expected payloads, got none")
	}

	// Should have 1 summary + 3 turn payloads
	if len(payloads) != 4 {
		t.Errorf("expected 4 payloads (1 summary + 3 turns), got %d", len(payloads))
	}

	// First should be summary
	if payloads[0].Type != "conversation_summary" {
		t.Errorf("first payload type = %q, want conversation_summary", payloads[0].Type)
	}

	// Check speaker contains openclaw session prefix
	if payloads[0].Speaker != "openclaw:abc12345" {
		t.Errorf("speaker = %q, want openclaw:abc12345", payloads[0].Speaker)
	}

	// Check tags contain source:openclaw
	foundSource := false
	for _, tag := range payloads[0].Tags {
		if tag == "source:openclaw" {
			foundSource = true
		}
	}
	if !foundSource {
		t.Error("expected source:openclaw tag")
	}
}

func TestOpenClawMarkdownPayload(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	soulFile := filepath.Join(wsDir, "SOUL.md")
	if err := os.WriteFile(soulFile, []byte("# SOUL.md\nI am Gilfoyle."), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Machine: MachineConfig{ID: "test-machine"},
	}
	agent := AgentConfig{
		Type:       "openclaw",
		Name:       "test-openclaw",
		Enabled:    true,
		Paths:      []string{dir},
		Include:    []string{"**/*.md"},
		Visibility: "internal",
	}
	privacy := PrivacyConfig{MaxFileSizeKB: 512}

	payloads, err := (openclawAdapter{}).Scan(cfg, agent, privacy)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}

	if payloads[0].Speaker != "openclaw-agent" {
		t.Errorf("speaker = %q, want openclaw-agent", payloads[0].Speaker)
	}

	if payloads[0].Project != "openclaw-workspace" {
		t.Errorf("project = %q, want openclaw-workspace", payloads[0].Project)
	}
}

func TestDetectOpenClawProject(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/.openclaw/agents/main/sessions/abc.jsonl", "openclaw-main"},
		{"/home/user/.openclaw/workspace/SOUL.md", "openclaw-workspace"},
		{"/home/user/.openclaw/workspace/memory/2026-01-01.md", "openclaw-workspace"},
		{"/home/user/.openclaw/random/file.md", "openclaw"},
	}

	for _, tt := range tests {
		got := detectOpenClawProject(tt.path)
		if got != tt.want {
			t.Errorf("detectOpenClawProject(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtractOpenClawContent(t *testing.T) {
	// String content
	got := extractOpenClawContent([]byte(`"hello world"`))
	if got != "hello world" {
		t.Errorf("string content = %q, want %q", got, "hello world")
	}

	// Array content
	got = extractOpenClawContent([]byte(`[{"type":"text","text":"part one"},{"type":"text","text":"part two"}]`))
	if got != "part one\npart two" {
		t.Errorf("array content = %q, want %q", got, "part one\npart two")
	}

	// Null/empty
	got = extractOpenClawContent(nil)
	if got != "" {
		t.Errorf("nil content = %q, want empty", got)
	}
}
