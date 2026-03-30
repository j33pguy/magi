package main

import (
	"encoding/json"
	"testing"
)

func TestGenerateConfig(t *testing.T) {
	cfg := generateConfig()

	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.MCPServers))
	}

	magi, ok := cfg.MCPServers["magi"]
	if !ok {
		t.Fatal("missing 'magi' server entry")
	}

	if magi.Command != "magi" {
		t.Errorf("command = %q, want %q", magi.Command, "magi")
	}

	if magi.Env["MAGI_GRPC_PORT"] != "8300" {
		t.Errorf("MAGI_GRPC_PORT = %q, want %q", magi.Env["MAGI_GRPC_PORT"], "8300")
	}

	// Verify it produces valid JSON.
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed mcpConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := parsed.MCPServers["magi"]; !ok {
		t.Error("round-trip lost 'magi' entry")
	}
}

func TestGenerateConfigEnvVars(t *testing.T) {
	cfg := generateConfig()
	magi := cfg.MCPServers["magi"]

	requiredEnvVars := []string{
		"MAGI_DB_URL",
		"MAGI_AUTH_TOKEN",
		"MAGI_API_TOKEN",
		"MAGI_GRPC_PORT",
		"MAGI_HTTP_PORT",
		"MAGI_LEGACY_HTTP_PORT",
		"MAGI_UI_PORT",
	}

	for _, env := range requiredEnvVars {
		if _, ok := magi.Env[env]; !ok {
			t.Errorf("missing env var %q", env)
		}
	}
}
