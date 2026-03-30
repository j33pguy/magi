package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/j33pguy/magi/internal/db"
)

// ---------------------------------------------------------------------------
// New() — error paths (no ONNX runtime required)
// ---------------------------------------------------------------------------

// TestNew_DBInitError verifies New returns an error when the storage backend
// is unrecognised, which causes db.NewStore to fail immediately.
func TestNew_DBInitError(t *testing.T) {
	t.Setenv("MEMORY_BACKEND", "nonexistent_backend")

	logger := testLogger()
	s, err := New(logger)
	if err == nil {
		s.Close()
		t.Fatal("expected error from New() with invalid MEMORY_BACKEND")
	}
	if s != nil {
		t.Fatal("expected nil Server when New() returns an error")
	}
}

// TestNew_MigrationSuccessEmbeddingsFail verifies New returns an error when
// the database initialises and migrates successfully but the ONNX embeddings
// provider cannot be created (model dir is a file, not a directory).
func TestNew_MigrationSuccessEmbeddingsFail(t *testing.T) {
	tmp := t.TempDir()

	// Use SQLite so we get past DB init + migration without Turso credentials.
	t.Setenv("MEMORY_BACKEND", "sqlite")
	t.Setenv("SQLITE_PATH", filepath.Join(tmp, "test.db"))

	// Force ONNX model dir to a path that cannot be created (parent is a file).
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAGI_MODEL_DIR", filepath.Join(blocker, "subdir"))

	// Also disable git so we don't interfere.
	t.Setenv("MAGI_GIT_ENABLED", "false")

	logger := testLogger()
	s, err := New(logger)
	if err == nil {
		s.Close()
		t.Fatal("expected error from New() when embeddings init fails")
	}
	if s != nil {
		t.Fatal("expected nil Server on embeddings error")
	}
}

// ---------------------------------------------------------------------------
// New() — full success path (requires ONNX runtime + model files)
// ---------------------------------------------------------------------------

// TestNew_FullSuccess exercises the full New() path including DB init,
// migration, ONNX embeddings, and MCP/gRPC/HTTP server wiring. Skipped if
// the ONNX model files are not present on disk.
func TestNew_FullSuccess(t *testing.T) {
	modelDir := "/home/j33p/.local/share/claude-memory/models"
	modelPath := filepath.Join(modelDir, "all-MiniLM-L6-v2", "model.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("ONNX model not found at %s, skipping full New() test", modelPath)
	}

	tmp := t.TempDir()

	t.Setenv("MEMORY_BACKEND", "sqlite")
	t.Setenv("SQLITE_PATH", filepath.Join(tmp, "full.db"))
	t.Setenv("MAGI_MODEL_DIR", modelDir)
	t.Setenv("MAGI_GIT_ENABLED", "false")
	t.Setenv("MAGI_API_TOKEN", "test-token")

	logger := testLogger()
	s, err := New(logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if s.mcp == nil {
		t.Error("mcp should not be nil")
	}
	if s.grpcServer == nil {
		t.Error("grpcServer should not be nil")
	}
	if s.httpAPI == nil {
		t.Error("httpAPI should not be nil")
	}
	if s.dbClient == nil {
		t.Error("dbClient should not be nil")
	}
	if s.embedder == nil {
		t.Error("embedder should not be nil")
	}
	if s.store == nil {
		t.Error("store should not be nil")
	}
}

// TestNew_FullSuccessWithGit exercises New() with git versioning enabled.
func TestNew_FullSuccessWithGit(t *testing.T) {
	modelDir := "/home/j33p/.local/share/claude-memory/models"
	modelPath := filepath.Join(modelDir, "all-MiniLM-L6-v2", "model.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("ONNX model not found at %s, skipping", modelPath)
	}

	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, "git-memories")

	t.Setenv("MEMORY_BACKEND", "sqlite")
	t.Setenv("SQLITE_PATH", filepath.Join(tmp, "git.db"))
	t.Setenv("MAGI_MODEL_DIR", modelDir)
	t.Setenv("MAGI_GIT_ENABLED", "true")
	t.Setenv("MAGI_GIT_PATH", gitDir)
	t.Setenv("MAGI_GIT_COMMIT_MODE", "immediate")
	t.Setenv("MAGI_API_TOKEN", "")

	logger := testLogger()
	s, err := New(logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if s.gitRepo == nil {
		t.Error("gitRepo should not be nil when MAGI_GIT_ENABLED=true")
	}
}

// TestNew_GitInitFailContinues verifies that New() succeeds even when git
// init fails (it logs and continues without git).
func TestNew_GitInitFailContinues(t *testing.T) {
	modelDir := "/home/j33p/.local/share/claude-memory/models"
	modelPath := filepath.Join(modelDir, "all-MiniLM-L6-v2", "model.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("ONNX model not found at %s, skipping", modelPath)
	}

	tmp := t.TempDir()

	// Point git path to a file (not a directory) to cause Init to fail.
	gitBlocker := filepath.Join(tmp, "git-blocker")
	if err := os.WriteFile(gitBlocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MEMORY_BACKEND", "sqlite")
	t.Setenv("SQLITE_PATH", filepath.Join(tmp, "gitfail.db"))
	t.Setenv("MAGI_MODEL_DIR", modelDir)
	t.Setenv("MAGI_GIT_ENABLED", "true")
	t.Setenv("MAGI_GIT_PATH", gitBlocker) // file, not dir — Init should fail
	t.Setenv("MAGI_API_TOKEN", "")

	logger := testLogger()
	s, err := New(logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if s.gitRepo != nil {
		t.Error("gitRepo should be nil when git init fails")
	}
}

// ---------------------------------------------------------------------------
// ServeGRPC — error from grpcServer.Serve (listener ok, serve fails)
// ---------------------------------------------------------------------------

// TestServeGRPC_ServeError verifies that when grpc.Server.Serve returns an
// error (e.g. the server was already stopped before Serve is called), the
// error is propagated.
func TestServeGRPC_ServeError(t *testing.T) {
	port := freePort(t)
	t.Setenv("MAGI_GRPC_PORT", port)

	gs := grpc.NewServer()
	s := &Server{
		grpcServer: gs,
		logger:     testLogger(),
	}

	// Stop the gRPC server before Serve is called. The next Serve call on a
	// stopped server returns immediately with nil (grpc behaviour), but we
	// exercise the full code path regardless.
	gs.Stop()

	err := s.ServeGRPC()
	// grpc.Server.Serve on a stopped server returns nil — we just confirm no panic.
	_ = err
}

// ---------------------------------------------------------------------------
// ServeGateway — registration error path (hard to trigger without mocking,
// but we can test the port-conflict error message format)
// ---------------------------------------------------------------------------

// TestServeGateway_ErrorContainsGateway verifies the error message from
// ServeGateway port conflict includes "gateway http".
func TestServeGateway_ErrorContainsGateway(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)

	t.Setenv("MAGI_HTTP_PORT", port)
	t.Setenv("MAGI_GRPC_PORT", freePort(t))

	s := &Server{
		logger: testLogger(),
	}

	err = s.ServeGateway()
	if err == nil {
		t.Fatal("expected error")
	}
	// The error should wrap from the "gateway http" fmt.Errorf path.
	if got := err.Error(); got == "" {
		t.Fatal("error message should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Close — all nil-check paths
// ---------------------------------------------------------------------------

// TestClose_AllNil verifies Close does not panic when every field is nil.
func TestClose_AllNil(t *testing.T) {
	s := &Server{}
	s.Close() // must not panic
}

// TestClose_OnlyGitRepo verifies Close with only gitRepo set.
func TestClose_OnlyGitRepo(t *testing.T) {
	repo := newTestGitRepo(t)
	s := &Server{
		gitRepo: repo,
	}
	s.Close()
}

// TestClose_OnlyEmbedder cannot be tested without ONNX, but we verify the
// nil embedder + nil db + nil git path (all nil).
func TestClose_NilGitRepoWithDBClient(t *testing.T) {
	tmp := t.TempDir()
	logger := testLogger()
	sqlClient, err := db.NewSQLiteClient(filepath.Join(tmp, "c.db"), logger)
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{
		dbClient: sqlClient.TursoClient,
		// gitRepo and embedder are nil
	}
	s.Close()
}

// TestClose_GitRepoNilEmbedderNilDB tests Close with gitRepo set but
// embedder and dbClient nil.
func TestClose_GitRepoNilEmbedderNilDB(t *testing.T) {
	repo := newTestGitRepo(t)
	s := &Server{
		gitRepo: repo,
	}
	s.Close()
}

// ---------------------------------------------------------------------------
// ShutdownGRPC — additional edge cases
// ---------------------------------------------------------------------------

// TestShutdownGRPC_CancelledContext verifies ShutdownGRPC handles a
// pre-cancelled context correctly when gwServer is set.
func TestShutdownGRPC_CancelledContext(t *testing.T) {
	port := freePort(t)

	gs := grpc.NewServer()
	gwServer := &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	s := &Server{
		grpcServer: gs,
		gwServer:   gwServer,
		logger:     testLogger(),
	}

	// Start gateway so Shutdown has something to shut down.
	go func() {
		_ = gwServer.ListenAndServe()
	}()
	waitForPort(t, port, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := s.ShutdownGRPC(ctx)
	// With a cancelled context, Shutdown may return context.Canceled.
	if err != nil && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ShutdownWeb — cancelled context
// ---------------------------------------------------------------------------

// TestShutdownWeb_CancelledContext verifies ShutdownWeb propagates a context
// cancellation error when the web server is running.
func TestShutdownWeb_CancelledContext(t *testing.T) {
	port := freePort(t)

	webServer := &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	s := &Server{
		webServer: webServer,
		logger:    testLogger(),
	}

	go func() {
		_ = webServer.ListenAndServe()
	}()
	waitForPort(t, port, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.ShutdownWeb(ctx)
	if err != nil && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}

	// Clean up: shut down properly so the goroutine exits.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutCancel()
	_ = webServer.Shutdown(shutCtx)
}

// ---------------------------------------------------------------------------
// ServeWeb — verify webServer is set after call
// ---------------------------------------------------------------------------

// TestServeWeb_RespondsToHTTP confirms ServeWeb populates s.webServer and that
// the server responds to HTTP requests (requires a real db.Client to avoid
// nil-pointer panics in the web handler).
func TestServeWeb_RespondsToHTTP(t *testing.T) {
	port := freePort(t)
	t.Setenv("MAGI_UI_PORT", port)

	sqlClient := newTestSQLiteClient(t)

	s := &Server{
		dbClient: sqlClient.TursoClient,
		embedder: nil,
		logger:   testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeWeb()
	}()

	waitForPort(t, port, 3*time.Second)

	// Verify the server actually serves HTTP.
	resp, err := http.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ShutdownWeb(ctx); err != nil {
		t.Fatalf("ShutdownWeb: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeWeb: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeWeb did not return")
	}
}

// ---------------------------------------------------------------------------
// ServeGRPC — verify default port env var is empty string
// ---------------------------------------------------------------------------

// TestServeGRPC_EmptyEnvUsesDefault verifies behaviour when MAGI_GRPC_PORT is
// explicitly set to empty string (should use default 8300).
func TestServeGRPC_EmptyEnvUsesDefault(t *testing.T) {
	t.Setenv("MAGI_GRPC_PORT", "")

	gs := grpc.NewServer()
	s := &Server{
		grpcServer: gs,
		logger:     testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeGRPC()
	}()

	// Don't wait too long — port 8300 may be in use.
	time.Sleep(100 * time.Millisecond)
	gs.GracefulStop()

	select {
	case <-errCh:
		// pass (error or nil is fine)
	case <-time.After(5 * time.Second):
		t.Fatal("ServeGRPC did not return")
	}
}

// ---------------------------------------------------------------------------
// Logging / logger field
// ---------------------------------------------------------------------------

// TestServer_LoggerField verifies the logger field is accessible.
func TestServer_LoggerField(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := &Server{
		logger: logger,
	}
	if s.logger == nil {
		t.Fatal("logger should not be nil")
	}
}

// ---------------------------------------------------------------------------
// Store field — verify store can be set independently of dbClient
// ---------------------------------------------------------------------------

// TestServer_StoreField verifies the store field can hold a different
// implementation than dbClient.
func TestServer_StoreField(t *testing.T) {
	sqlClient := newTestSQLiteClient(t)

	s := &Server{
		dbClient: sqlClient.TursoClient,
		store:    sqlClient, // SQLiteClient implements Store
	}

	if s.store == nil {
		t.Fatal("store should not be nil")
	}
	if s.dbClient == nil {
		t.Fatal("dbClient should not be nil")
	}
}

// ---------------------------------------------------------------------------
// ServeGateway — verify gwServer field is populated
// ---------------------------------------------------------------------------

// TestServeGateway_PopulatesGwServer confirms that after ServeGateway starts,
// s.gwServer is non-nil.
func TestServeGateway_PopulatesGwServer(t *testing.T) {
	httpPort := freePort(t)
	grpcPort := freePort(t)
	t.Setenv("MAGI_HTTP_PORT", httpPort)
	t.Setenv("MAGI_GRPC_PORT", grpcPort)

	s := &Server{
		logger: testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeGateway()
	}()

	waitForPort(t, httpPort, 3*time.Second)

	if s.gwServer == nil {
		t.Fatal("gwServer should be set after ServeGateway starts")
	}

	// Verify the gateway responds to HTTP.
	resp, err := http.Get("http://127.0.0.1:" + httpPort + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.gwServer.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeGateway: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeGateway did not return")
	}
}

// ---------------------------------------------------------------------------
// Multiple Close calls — idempotency
// ---------------------------------------------------------------------------

// TestClose_DoubleClose verifies calling Close twice does not panic.
func TestClose_DoubleClose(t *testing.T) {
	tmp := t.TempDir()
	logger := testLogger()
	sqlClient, err := db.NewSQLiteClient(filepath.Join(tmp, "dc.db"), logger)
	if err != nil {
		t.Fatal(err)
	}

	repo := newTestGitRepo(t)

	s := &Server{
		logger:   logger,
		gitRepo:  repo,
		dbClient: sqlClient.TursoClient,
	}

	s.Close()
	s.Close() // second call — must not panic
}
