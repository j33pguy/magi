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

	if len(magi.Args) != 1 || magi.Args[0] != "--mcp-only" {
		t.Errorf("args = %#v, want [--mcp-only]", magi.Args)
	}

	if magi.Env["MAGI_CACHE_ENABLED"] != "true" {
		t.Errorf("MAGI_CACHE_ENABLED = %q, want %q", magi.Env["MAGI_CACHE_ENABLED"], "true")
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
		"MEMORY_BACKEND",
		"SQLITE_PATH",
		"POSTGRES_URL",
		"MYSQL_DSN",
		"SQLSERVER_URL",
		"TURSO_URL",
		"TURSO_AUTH_TOKEN",
		"MAGI_REPLICA_PATH",
		"MAGI_API_TOKEN",
		"MAGI_ASYNC_WRITES",
		"MAGI_CACHE_ENABLED",
		"MAGI_UI_ENABLED",
	}

	for _, env := range requiredEnvVars {
		if _, ok := magi.Env[env]; !ok {
			t.Errorf("missing env var %q", env)
		}
	}
}
