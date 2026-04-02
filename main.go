// Package main is the entry point for the magi MCP server.
// MAGI - Multi-Agent Graph Intelligence. Universal memory server for AI agents.
// (distributed libSQL with vector search) and local ONNX embeddings.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/j33pguy/magi/internal/server"
)

func main() {
	// mcp-config: output MCP JSON config and exit
	if len(os.Args) > 1 && os.Args[1] == "mcp-config" {
		printMCPConfig()
		return
	}

	// --http-only: run HTTP/gRPC servers only (no stdio MCP). Used for systemd deployments.
	httpOnly := len(os.Args) > 1 && os.Args[1] == "--http-only"

	// --mcp-only: run ONLY the MCP stdio server (no HTTP/gRPC/web). Used for Codex/Claude MCP integration.
	mcpOnly := len(os.Args) > 1 && os.Args[1] == "--mcp-only"

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	s, err := server.New(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize server: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	// Graceful shutdown on SIGTERM/SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Start gRPC server (all modes)
	go func() {
		if err := s.ServeGRPC(); err != nil {
			logger.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start grpc-gateway HTTP proxy (all modes)
	go func() {
		if err := s.ServeGateway(); err != nil {
			logger.Error("grpc-gateway server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start web UI server (all modes)
	go func() {
		if err := s.ServeWeb(); err != nil {
			logger.Error("Web UI server error", "error", err)
		}
	}()

	if mcpOnly {
		// MCP-only mode: just stdio MCP, no network listeners
		logger.Info("Starting in MCP-only mode (stdio only)")
		go func() {
			<-sigCh
			logger.Info("Received shutdown signal")
		}()
		if err := s.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if httpOnly {
		// HTTP-only mode: serve legacy HTTP API alongside gRPC and block on signal
		logger.Info("Starting in HTTP-only mode (gRPC + gateway + legacy HTTP + web UI)")
		go func() {
			if err := s.ServeHTTP(); err != nil {
				logger.Error("HTTP API server error", "error", err)
				os.Exit(1)
			}
		}()
		<-sigCh
		logger.Info("Received shutdown signal")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.ShutdownGRPC(ctx); err != nil {
			logger.Error("gRPC shutdown error", "error", err)
		}
		if err := s.ShutdownHTTP(ctx); err != nil {
			logger.Error("HTTP shutdown error", "error", err)
		}
		if err := s.ShutdownWeb(ctx); err != nil {
			logger.Error("Web UI shutdown error", "error", err)
		}
		return
	}

	// Default: MCP stdio mode — gRPC, gateway, and legacy HTTP run in background
	go func() {
		if err := s.ServeHTTP(); err != nil {
			logger.Error("HTTP API server error", "error", err)
		}
	}()

	go func() {
		<-sigCh
		logger.Info("Received shutdown signal")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.ShutdownGRPC(ctx); err != nil {
			logger.Error("gRPC shutdown error", "error", err)
		}
		if err := s.ShutdownHTTP(ctx); err != nil {
			logger.Error("HTTP shutdown error", "error", err)
		}
		if err := s.ShutdownWeb(ctx); err != nil {
			logger.Error("Web UI shutdown error", "error", err)
		}
	}()

	// Run MCP server on stdio (blocks until stdin closes)
	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// printMCPConfig outputs a valid MCP JSON config block for Claude/Codex integration.
func printMCPConfig() {
	type mcpServerEntry struct {
		Command string            `json:"command"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}
	cfg := map[string]any{
		"mcpServers": map[string]mcpServerEntry{
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
