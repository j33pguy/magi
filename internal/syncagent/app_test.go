package syncagent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
