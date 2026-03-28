// Package main is the entry point for the claude-memory MCP server.
// It provides a RAG-based memory system for Claude Code using Turso
// (distributed libSQL with vector search) and local ONNX embeddings.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/j33pguy/claude-memory/server"
)

func main() {
	// --http-only: run HTTP/gRPC servers only (no stdio MCP). Used for systemd deployments.
	httpOnly := len(os.Args) > 1 && os.Args[1] == "--http-only"

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
