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

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/metrics"
	"github.com/j33pguy/magi/internal/pipeline"
	"github.com/j33pguy/magi/internal/secretstore"
	"github.com/j33pguy/magi/internal/vcs"
)

// Server is the HTTP API server for magi.
type Server struct {
	httpServer *http.Server
	db         db.Store
	tasks      TaskStore
	embedder   embeddings.Provider
	logger     *slog.Logger
	auth       *auth.Resolver
	machines   MachineRegistryStore
	enrollment EnrollmentStore
	secrets    secretstore.Manager
	gitRepo    *vcs.Repo        // optional — nil if git versioning is disabled
	pipeline   *pipeline.Writer // optional — nil if async writes disabled
}

// MachineRegistryStore manages machine credentials for sync and worker auth.
type MachineRegistryStore interface {
	CreateMachineCredential(cred *db.MachineCredential) (*db.MachineCredential, error)
	ListMachineCredentials() ([]*db.MachineCredential, error)
	RevokeMachineCredential(id string) error
}

// EnrollmentStore manages one-time enrollment tokens.
type EnrollmentStore interface {
	CreateEnrollmentToken(et *db.EnrollmentToken) (*db.EnrollmentToken, error)
	GetEnrollmentTokenByHash(tokenHash string) (*db.EnrollmentToken, error)
	IncrementEnrollmentTokenUse(id string) error
	ListEnrollmentTokens() ([]*db.EnrollmentToken, error)
	RevokeEnrollmentToken(id string) error
}

// TaskStore manages a separate task queue outside the memory stack.
type TaskStore interface {
	db.TaskQueueStore
}

// NewServer creates a new HTTP API server.
func NewServer(dbClient db.Store, embedder embeddings.Provider, logger *slog.Logger) *Server {
	s := &Server{
		db:       dbClient,
		embedder: embedder,
		logger:   logger,
	}
	resolver, err := auth.LoadResolverFromEnv()
	if err != nil {
		logger.Error("loading auth config failed, falling back to admin token only", "error", err)
		resolver = &auth.Resolver{}
	}
	if lookup, ok := dbClient.(auth.MachineLookup); ok {
		resolver.SetMachineLookup(lookup)
	}
	s.auth = resolver
	if machines, ok := dbClient.(MachineRegistryStore); ok {
		s.machines = machines
	}
	if enrollment, ok := dbClient.(EnrollmentStore); ok {
		s.enrollment = enrollment
	}
	secretManager, err := secretstore.NewFromEnv(logger)
	if err != nil {
		logger.Error("loading secret store config failed", "error", err)
	}
	s.secrets = secretManager

	port := os.Getenv("MAGI_LEGACY_HTTP_PORT")
	if port == "" {
		port = "8302"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /livez", s.handleLivez)
	metrics.RegisterRoutes(mux)
	mux.HandleFunc("POST /recall", s.requireAuth(s.handleRecall))
	mux.HandleFunc("POST /remember", s.requireAuth(s.handleRemember))
	mux.HandleFunc("POST /memory", s.requireAuth(s.handleRemember))
	mux.HandleFunc("POST /memory/recall", s.requireAuth(s.handleRecall))
	mux.HandleFunc("GET /memory", s.requireAuth(s.handleListMemories))
	mux.HandleFunc("GET /memory/search", s.requireAuth(s.handleSearch))
	mux.HandleFunc("DELETE /memory/{id}", s.requireAuth(s.handleDeleteMemory))
	mux.HandleFunc("GET /memory/{id}/history", s.requireAuth(s.handleMemoryHistory))
	mux.HandleFunc("GET /memory/{id}/diff", s.requireAuth(s.handleMemoryDiff))
	mux.HandleFunc("GET /memory/{id}/status", s.requireAuth(s.handleMemoryStatus))
	mux.HandleFunc("POST /sync/memories", s.requireAuth(s.handleSyncRemember))
	mux.HandleFunc("POST /memory/sync", s.requireAuth(s.handleSyncRemember))
	mux.HandleFunc("GET /memories", s.requireAuth(s.handleListMemories))
	mux.HandleFunc("DELETE /memories/{id}", s.requireAuth(s.handleDeleteMemory))
	mux.HandleFunc("GET /search", s.requireAuth(s.handleSearch))
	mux.HandleFunc("GET /patterns", s.requireAuth(s.handleListPatterns))
	mux.HandleFunc("GET /patterns/trending", s.requireAuth(s.handleListTrendingPatterns))
	mux.HandleFunc("POST /conversations", s.requireAuth(s.handleCreateConversation))
	mux.HandleFunc("GET /conversations", s.requireAuth(s.handleListConversations))
	mux.HandleFunc("POST /conversations/search", s.requireAuth(s.handleSearchConversations))
	mux.HandleFunc("GET /conversations/{id}", s.requireAuth(s.handleGetConversation))
	mux.HandleFunc("POST /tasks", s.requireAuth(s.handleCreateTask))
	mux.HandleFunc("GET /tasks", s.requireAuth(s.handleListTasks))
	mux.HandleFunc("GET /tasks/{id}", s.requireAuth(s.handleGetTask))
	mux.HandleFunc("PATCH /tasks/{id}", s.requireAuth(s.handleUpdateTask))
	mux.HandleFunc("POST /tasks/{id}/events", s.requireAuth(s.handleCreateTaskEvent))
	mux.HandleFunc("GET /tasks/{id}/events", s.requireAuth(s.handleListTaskEvents))
	mux.HandleFunc("POST /task", s.requireAuth(s.handleCreateTask))
	mux.HandleFunc("GET /task", s.requireAuth(s.handleListTasks))
	mux.HandleFunc("GET /task/{id}", s.requireAuth(s.handleGetTask))
	mux.HandleFunc("PATCH /task/{id}", s.requireAuth(s.handleUpdateTask))
	mux.HandleFunc("POST /task/{id}/event", s.requireAuth(s.handleCreateTaskEvent))
	mux.HandleFunc("GET /task/{id}/event", s.requireAuth(s.handleListTaskEvents))
	mux.HandleFunc("POST /task/{id}/events", s.requireAuth(s.handleCreateTaskEvent))
	mux.HandleFunc("GET /task/{id}/events", s.requireAuth(s.handleListTaskEvents))
	mux.HandleFunc("GET /memories/{id}/history", s.requireAuth(s.handleMemoryHistory))
	mux.HandleFunc("GET /memories/{id}/diff", s.requireAuth(s.handleMemoryDiff))
	mux.HandleFunc("GET /memories/{id}/status", s.requireAuth(s.handleMemoryStatus))
	mux.HandleFunc("GET /pipeline/stats", s.requireAuth(s.handlePipelineStats))
	mux.HandleFunc("POST /auth/machines/enroll", s.requireAuth(s.handleEnrollMachine))
	mux.HandleFunc("GET /auth/machines", s.requireAuth(s.handleListMachineCredentials))
	mux.HandleFunc("POST /auth/machines/{id}/revoke", s.requireAuth(s.handleRevokeMachineCredential))

	// Enrollment token management (admin-only)
	mux.HandleFunc("POST /auth/enrollment-tokens", s.requireAuth(s.handleCreateEnrollmentToken))
	mux.HandleFunc("GET /auth/enrollment-tokens", s.requireAuth(s.handleListEnrollmentTokens))
	mux.HandleFunc("POST /auth/enrollment-tokens/{id}/revoke", s.requireAuth(s.handleRevokeEnrollmentToken))

	// Self-enrollment: unauthenticated, burns enrollment token
	mux.HandleFunc("POST /auth/enroll", s.handleSelfEnroll)
	mux.HandleFunc("POST /auth/secrets/resolve", s.requireAuth(s.handleResolveSecret))

	s.httpServer = &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Start begins listening for HTTP requests. Blocks until the server stops.
func (s *Server) Start() error {
	if s.auth == nil || !s.auth.Enabled() {
		s.logger.Warn("no auth configured, running in read-only dev mode")
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

func (s *Server) SetAuthResolver(resolver *auth.Resolver) {
	if resolver != nil {
		s.auth = resolver
	}
}

func (s *Server) SetMachineStore(machines MachineRegistryStore) {
	s.machines = machines
}

// SetEnrollmentStore configures the enrollment token backend.
func (s *Server) SetEnrollmentStore(enrollment EnrollmentStore) {
	s.enrollment = enrollment
}

func (s *Server) SetTaskStore(tasks TaskStore) {
	s.tasks = tasks
}

func (s *Server) SetSecretManager(manager secretstore.Manager) {
	s.secrets = manager
}
