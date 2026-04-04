package syncagent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAppEnrollPersistsMachineToken(t *testing.T) {
	var gotAuth string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/machines/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"token":"machine-secret","record":{"id":"cred-1","user":"UserA","machine_id":"MachineA","groups":["platform"]}}`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{
		Server: ServerConfig{
			URL:         server.URL,
			EnrollToken: "admin-secret",
			Protocol:    "http",
		},
		Machine: MachineConfig{
			ID:     "MachineA",
			User:   "UserA",
			Groups: []string{"platform"},
		},
		Sync: SyncConfig{
			StateFile: filepath.Join(t.TempDir(), "state.json"),
		},
		Privacy: PrivacyConfig{
			Mode: "allowlist",
		},
		Agents: []AgentConfig{
			{Type: "claude", Enabled: false, ViewerGroups: []string{"platform"}},
		},
	}
	if err := SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	app, err := New(loaded, configPath, NewLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := app.Run(context.Background(), ModeEnroll); err != nil {
		t.Fatalf("Run enroll: %v", err)
	}

	if gotAuth != "Bearer admin-secret" {
		t.Fatalf("auth = %q want Bearer admin-secret", gotAuth)
	}
	if !strings.Contains(gotBody, `"machine_id":"MachineA"`) || !strings.Contains(gotBody, `"user":"UserA"`) {
		t.Fatalf("unexpected body %q", gotBody)
	}
	if !strings.Contains(gotBody, `"groups":["platform"]`) {
		t.Fatalf("expected machine groups in body, got %q", gotBody)
	}

	reloaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig after enroll: %v", err)
	}
	if reloaded.Server.Token != "machine-secret" {
		t.Fatalf("server.token = %q want machine-secret", reloaded.Server.Token)
	}
	if reloaded.Server.EnrollToken != "" {
		t.Fatalf("server.enroll_token = %q want empty", reloaded.Server.EnrollToken)
	}
}

func TestWatchTriggersSync(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watch test in short mode")
	}

	var syncCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/memories" {
			syncCount.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	watchDir := t.TempDir()
	stateFile := filepath.Join(t.TempDir(), "state.json")

	// Create a markdown file before starting watch so initial sync finds it.
	if err := os.WriteFile(filepath.Join(watchDir, "CLAUDE.md"), []byte("# project context"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Server:  ServerConfig{URL: server.URL, Token: "tok", Protocol: "http"},
		Machine: MachineConfig{ID: "test", User: "tester"},
		Sync:    SyncConfig{Mode: "push", Interval: "1s", StateFile: stateFile},
		Privacy: PrivacyConfig{Mode: "allowlist"},
		Agents: []AgentConfig{
			{
				Name:    "claude",
				Type:    "claude",
				Enabled: true,
				Paths:   []string{watchDir},
				Include: []string{"**.md"},
			},
		},
	}

	app, err := New(cfg, filepath.Join(t.TempDir(), "config.yaml"), NewLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx, ModeWatch) }()

	// Wait for initial sync to complete.
	time.Sleep(200 * time.Millisecond)
	initialCount := syncCount.Load()
	if initialCount == 0 {
		t.Fatal("expected initial sync to upload at least one file")
	}

	// Write a new file to trigger a watch event + debounced sync.
	if err := os.WriteFile(filepath.Join(watchDir, "notes.md"), []byte("# new notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce (500ms) + sync time.
	time.Sleep(1500 * time.Millisecond)

	afterCount := syncCount.Load()
	if afterCount <= initialCount {
		t.Fatalf("expected sync after file change: initial=%d after=%d", initialCount, afterCount)
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("watch returned unexpected error: %v", err)
	}
}

func TestWatchGracefulShutdown(t *testing.T) {
	watchDir := t.TempDir()
	stateFile := filepath.Join(t.TempDir(), "state.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		Server:  ServerConfig{URL: server.URL, Token: "tok", Protocol: "http"},
		Machine: MachineConfig{ID: "test", User: "tester"},
		Sync:    SyncConfig{Mode: "push", Interval: "1s", StateFile: stateFile},
		Privacy: PrivacyConfig{Mode: "allowlist"},
		Agents: []AgentConfig{
			{
				Name:    "claude",
				Type:    "claude",
				Enabled: true,
				Paths:   []string{watchDir},
			},
		},
	}

	app, err := New(cfg, filepath.Join(t.TempDir(), "config.yaml"), NewLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx, ModeWatch) }()

	// Give watch time to start.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not shut down within 3 seconds")
	}
}

func TestMatchesAgent(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(t.TempDir(), "state.json")

	cfg := &Config{
		Server:  ServerConfig{URL: "http://localhost", Protocol: "http"},
		Machine: MachineConfig{ID: "test", User: "tester"},
		Sync:    SyncConfig{Mode: "push", StateFile: stateFile},
		Privacy: PrivacyConfig{Mode: "allowlist"},
		Agents: []AgentConfig{
			{
				Name:    "claude",
				Type:    "claude",
				Enabled: true,
				Paths:   []string{dir},
				Include: []string{"**.md"},
				Exclude: []string{"secret/**"},
			},
		},
	}

	app, err := New(cfg, filepath.Join(t.TempDir(), "config.yaml"), NewLogger())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "README.md"), true},
		{filepath.Join(dir, "sub/notes.md"), true},
		{filepath.Join(dir, "code.go"), false},
		{filepath.Join(dir, "secret/private.md"), false},
		{"/outside/path/file.md", false},
	}

	for _, tt := range tests {
		got := app.matchesAgent(tt.path)
		if got != tt.want {
			t.Errorf("matchesAgent(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
