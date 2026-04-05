package syncagent

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudePath := filepath.Join(home, ".claude")
	openclawPath := filepath.Join(home, ".openclaw")
	if err := os.MkdirAll(claudePath, 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	if err := os.MkdirAll(openclawPath, 0o755); err != nil {
		t.Fatalf("mkdir openclaw: %v", err)
	}

	agents := DetectAgents()
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}
	if !agents[0].Found || agents[0].Type != "claude" {
		t.Fatalf("expected claude found, got %+v", agents[0])
	}
	if !agents[1].Found || agents[1].Type != "openclaw" {
		t.Fatalf("expected openclaw found, got %+v", agents[1])
	}
	if agents[2].Found || agents[2].Type != "codex" {
		t.Fatalf("expected codex not found, got %+v", agents[2])
	}
}

func TestRunInteractiveWritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USER", "test-user")

	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".openclaw"), 0o755); err != nil {
		t.Fatalf("mkdir openclaw: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configPath := filepath.Join(home, "config.yaml")
	input := strings.Join([]string{
		server.URL,
		"y",
		"enroll-123",
		"test-machine",
		"test-user",
		"y",
		"n",
		"",
		"",
		"n",
		"n",
		"",
	}, "\n")

	var output bytes.Buffer
	if err := RunInteractive(context.Background(), configPath, strings.NewReader(input), &output, nil); err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.URL != server.URL {
		t.Fatalf("server url = %q", cfg.Server.URL)
	}
	if cfg.Server.EnrollToken != "enroll-123" {
		t.Fatalf("enroll token = %q", cfg.Server.EnrollToken)
	}
	if cfg.Server.Token != "" {
		t.Fatalf("machine token should be empty")
	}
	if cfg.Machine.ID != "test-machine" {
		t.Fatalf("machine id = %q", cfg.Machine.ID)
	}
	if cfg.Machine.User != "test-user" {
		t.Fatalf("machine user = %q", cfg.Machine.User)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(cfg.Agents))
	}
	if cfg.Agents[0].Type != "claude" {
		t.Fatalf("agent type = %q", cfg.Agents[0].Type)
	}
	if cfg.Privacy.Mode != "allowlist" {
		t.Fatalf("privacy mode = %q", cfg.Privacy.Mode)
	}
	if !cfg.Privacy.RedactSecrets {
		t.Fatalf("expected redact_secrets true")
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config perm = %v", info.Mode().Perm())
	}
}

func TestRunInteractiveKeepsExistingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(configPath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var output bytes.Buffer
	if err := RunInteractive(context.Background(), configPath, strings.NewReader("n\n"), &output, nil); err != nil {
		t.Fatalf("run interactive: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("config changed unexpectedly")
	}
}
