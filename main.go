// Package main is the entry point for the claude-memory MCP server.
// It provides a RAG-based memory system for Claude Code using Turso
// (distributed libSQL with vector search) and local ONNX embeddings.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/russseaman/claude-memory/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	s, err := server.New(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize server: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
