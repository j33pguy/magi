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
				Args:    []string{},
				Env: map[string]string{
					"MAGI_DB_URL":          "${MAGI_DB_URL}",
					"MAGI_AUTH_TOKEN":      "${MAGI_AUTH_TOKEN}",
					"MAGI_API_TOKEN":       "${MAGI_API_TOKEN}",
					"MAGI_GRPC_PORT":       "8300",
					"MAGI_HTTP_PORT":       "8301",
					"MAGI_LEGACY_HTTP_PORT": "8302",
					"MAGI_UI_PORT":         "8080",
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
