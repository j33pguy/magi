// Package api provides an HTTP API layer for magi.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// Server is the HTTP API server for magi.
type Server struct {
	httpServer *http.Server
	db         *db.Client
	embedder   embeddings.Provider
	logger     *slog.Logger
	token      string
}

// NewServer creates a new HTTP API server.
func NewServer(dbClient *db.Client, embedder embeddings.Provider, logger *slog.Logger) *Server {
	s := &Server{
		db:       dbClient,
		embedder: embedder,
		logger:   logger,
		token:    os.Getenv("MAGI_API_TOKEN"),
	}

	port := os.Getenv("MAGI_LEGACY_HTTP_PORT")
	if port == "" {
		port = "8302"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /recall", s.requireAuth(s.handleRecall))
	mux.HandleFunc("POST /remember", s.requireAuth(s.handleRemember))
	mux.HandleFunc("GET /memories", s.requireAuth(s.handleListMemories))
	mux.HandleFunc("DELETE /memories/{id}", s.requireAuth(s.handleDeleteMemory))
	mux.HandleFunc("GET /search", s.requireAuth(s.handleSearch))
	mux.HandleFunc("POST /conversations", s.requireAuth(s.handleCreateConversation))
	mux.HandleFunc("GET /conversations", s.requireAuth(s.handleListConversations))
	mux.HandleFunc("POST /conversations/search", s.requireAuth(s.handleSearchConversations))
	mux.HandleFunc("GET /conversations/{id}", s.requireAuth(s.handleGetConversation))

	s.httpServer = &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Start begins listening for HTTP requests. Blocks until the server stops.
func (s *Server) Start() error {
	if s.token == "" {
		s.logger.Warn("MAGI_API_TOKEN not set, running without auth (dev mode)")
	}
	s.logger.Info("Starting HTTP API server", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP API server")
	return s.httpServer.Shutdown(ctx)
}
