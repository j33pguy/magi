// Command mcp-config outputs a valid MCP JSON config block for Claude/Codex integration.
//
// Usage: go run ./cmd/mcp-config
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func generateConfig() mcpConfig {
	return mcpConfig{
		MCPServers: map[string]mcpServerEntry{
			"magi": {
				Command: "magi",
				Args:    []string{"--mcp-only"},
				Env: map[string]string{
					"MEMORY_BACKEND":     "${MEMORY_BACKEND}",
					"SQLITE_PATH":        "${SQLITE_PATH}",
					"POSTGRES_URL":       "${POSTGRES_URL}",
					"MYSQL_DSN":          "${MYSQL_DSN}",
					"SQLSERVER_URL":      "${SQLSERVER_URL}",
					"TURSO_URL":          "${TURSO_URL}",
					"TURSO_AUTH_TOKEN":   "${TURSO_AUTH_TOKEN}",
					"MAGI_REPLICA_PATH":  "${MAGI_REPLICA_PATH}",
					"MAGI_API_TOKEN":     "${MAGI_API_TOKEN}",
					"MAGI_ASYNC_WRITES":  "true",
					"MAGI_CACHE_ENABLED": "true",
					"MAGI_UI_ENABLED":    "false",
				},
			},
		},
	}
}

func run() error {
	cfg := generateConfig()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
