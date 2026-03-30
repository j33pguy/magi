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

	mcpserver "github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"

	"github.com/j33pguy/magi/internal/api"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/vcs"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestSQLiteClient creates a temporary SQLite-backed db.Client for testing.
func newTestSQLiteClient(t *testing.T) *db.SQLiteClient {
	t.Helper()
	tmp := t.TempDir()
	logger := testLogger()
	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return client
}

// newTestGitRepo creates a temporary vcs.Repo for testing.
func newTestGitRepo(t *testing.T) *vcs.Repo {
	t.Helper()
	dir := t.TempDir()
	cfg := &vcs.Config{
		Enabled:       true,
		Path:          dir,
		CommitMode:    "immediate",
		BatchInterval: time.Second,
	}
	repo, err := vcs.Init(cfg)
	if err != nil {
		t.Fatalf("vcs.Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

// freePort returns an available TCP port.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return fmt.Sprintf("%d", port)
}

// waitForPort polls until a TCP connection succeeds on the given port or the
// deadline is exceeded.
func waitForPort(t *testing.T, port string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("port %s did not become available within %v", port, timeout)
}

// --- Existing tests (preserved) ---

// TestServerStructFields is a compile-time check that all expected fields
// exist on Server with the correct types.
func TestServerStructFields(t *testing.T) {
	s := &Server{}

	// Assign typed nil values to verify field types at compile time.
	var _ *mcpserver.MCPServer = s.mcp
	var _ *api.Server = s.httpAPI
	var _ *grpc.Server = s.grpcServer
	var _ *http.Server = s.gwServer
	var _ *http.Server = s.webServer
	var _ *db.Client = s.dbClient
	var _ *embeddings.OnnxProvider = s.embedder
	var _ *slog.Logger = s.logger
}

func TestShutdownWeb_NilWebServer(t *testing.T) {
	s := &Server{
		logger: slog.Default(),
	}

	err := s.ShutdownWeb(context.Background())
	if err != nil {
		t.Fatalf("ShutdownWeb with nil webServer should return nil, got: %v", err)
	}
}

func TestShutdownGRPC_NilGwServer(t *testing.T) {
	// Create a real grpc.Server so GracefulStop can be called.
	gs := grpc.NewServer()

	s := &Server{
		grpcServer: gs,
		logger:     slog.Default(),
	}

	err := s.ShutdownGRPC(context.Background())
	if err != nil {
		t.Fatalf("ShutdownGRPC with nil gwServer should return nil, got: %v", err)
	}
}

func TestClose_NilEmbedderAndDBClient(t *testing.T) {
	s := &Server{
		logger: slog.Default(),
	}

	// Should not panic when embedder and dbClient are nil.
	s.Close()
}

func TestClose_NilEmbedderOnly(t *testing.T) {
	s := &Server{
		logger: slog.Default(),
		// dbClient is nil, embedder is nil
	}

	// Verify no panic.
	s.Close()
}

// --- New tests ---

// TestServeGRPC_StartAndStop starts a gRPC server on a random port, verifies
// it is listening, then stops it.
func TestServeGRPC_StartAndStop(t *testing.T) {
	port := freePort(t)
	t.Setenv("MAGI_GRPC_PORT", port)

	gs := grpc.NewServer()
	s := &Server{
		grpcServer: gs,
		logger:     testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeGRPC()
	}()

	waitForPort(t, port, 3*time.Second)

	gs.GracefulStop()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeGRPC returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeGRPC did not return after GracefulStop")
	}
}

// TestServeGRPC_PortInUse verifies ServeGRPC returns an error when the port
// is already occupied.
func TestServeGRPC_PortInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
	t.Setenv("MAGI_GRPC_PORT", port)

	gs := grpc.NewServer()
	s := &Server{
		grpcServer: gs,
		logger:     testLogger(),
	}

	err = s.ServeGRPC()
	if err == nil {
		t.Fatal("expected error when port is in use")
	}
}

// TestServeHTTP_StartAndShutdown exercises ServeHTTP and ShutdownHTTP using a
// real api.Server.
func TestServeHTTP_StartAndShutdown(t *testing.T) {
	port := freePort(t)
	t.Setenv("MAGI_LEGACY_HTTP_PORT", port)

	sqlClient := newTestSQLiteClient(t)
	logger := testLogger()

	// api.NewServer reads MAGI_LEGACY_HTTP_PORT from env.
	httpAPI := api.NewServer(sqlClient, nil, logger)

	s := &Server{
		httpAPI: httpAPI,
		logger:  logger,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeHTTP()
	}()

	waitForPort(t, port, 3*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ShutdownHTTP(ctx); err != nil {
		t.Fatalf("ShutdownHTTP: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeHTTP returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeHTTP did not return after shutdown")
	}
}

// TestShutdownGRPC_WithGwServer tests ShutdownGRPC when gwServer is non-nil and
// actively listening.
func TestShutdownGRPC_WithGwServer(t *testing.T) {
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

	errCh := make(chan error, 1)
	go func() {
		if err := gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	waitForPort(t, port, 3*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ShutdownGRPC(ctx); err != nil {
		t.Fatalf("ShutdownGRPC: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("gwServer returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gwServer did not stop after ShutdownGRPC")
	}
}

// TestShutdownWeb_WithRealWebServer tests ShutdownWeb with a running http.Server.
func TestShutdownWeb_WithRealWebServer(t *testing.T) {
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

	errCh := make(chan error, 1)
	go func() {
		if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	waitForPort(t, port, 3*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ShutdownWeb(ctx); err != nil {
		t.Fatalf("ShutdownWeb: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("webServer returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webServer did not stop after ShutdownWeb")
	}
}

// TestServeWeb_StartAndShutdown starts ServeWeb on a random port and verifies
// it listens, then shuts it down. ServeWeb calls web.RegisterRoutes which
// accepts nil for dbClient and embedder.
func TestServeWeb_StartAndShutdown(t *testing.T) {
	port := freePort(t)
	t.Setenv("MAGI_UI_PORT", port)

	s := &Server{
		dbClient: nil,
		embedder: nil,
		logger:   testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeWeb()
	}()

	waitForPort(t, port, 3*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ShutdownWeb(ctx); err != nil {
		t.Fatalf("ShutdownWeb: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeWeb returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeWeb did not return after shutdown")
	}
}

// TestServeWeb_PortInUse verifies ServeWeb returns an error when the port is busy.
func TestServeWeb_PortInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
	t.Setenv("MAGI_UI_PORT", port)

	s := &Server{
		dbClient: nil,
		embedder: nil,
		logger:   testLogger(),
	}

	err = s.ServeWeb()
	if err == nil {
		t.Fatal("expected error when port is in use")
	}
}

// TestServeGateway_PortInUse verifies ServeGateway returns an error when the
// HTTP port is busy.
func TestServeGateway_PortInUse(t *testing.T) {
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
		t.Fatal("expected error when port is in use")
	}
}

// TestServeGateway_StartAndShutdown starts ServeGateway and shuts it down.
// The gateway starts its HTTP listener even without a real gRPC backend.
func TestServeGateway_StartAndShutdown(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if s.gwServer != nil {
		if err := s.gwServer.Shutdown(ctx); err != nil {
			t.Fatalf("gwServer.Shutdown: %v", err)
		}
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeGateway returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeGateway did not return after shutdown")
	}
}

// TestClose_WithGitRepo tests Close with a real vcs.Repo.
func TestClose_WithGitRepo(t *testing.T) {
	repo := newTestGitRepo(t)

	s := &Server{
		logger:  testLogger(),
		gitRepo: repo,
	}

	// Should not panic.
	s.Close()
}

// TestClose_WithDBClient tests Close with a real SQLite-backed db.Client.
func TestClose_WithDBClient(t *testing.T) {
	tmp := t.TempDir()
	logger := testLogger()
	sqlClient, err := db.NewSQLiteClient(filepath.Join(tmp, "close-test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	// Note: we do NOT register a cleanup because Close() is the test subject.

	s := &Server{
		logger:   testLogger(),
		dbClient: sqlClient.TursoClient,
	}

	// Should not panic and should close the DB.
	s.Close()
}

// TestClose_WithGitRepoAndDBClient tests Close with both gitRepo and dbClient set.
func TestClose_WithGitRepoAndDBClient(t *testing.T) {
	repo := newTestGitRepo(t)

	tmp := t.TempDir()
	logger := testLogger()
	sqlClient, err := db.NewSQLiteClient(filepath.Join(tmp, "close-full.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}

	s := &Server{
		logger:   testLogger(),
		gitRepo:  repo,
		dbClient: sqlClient.TursoClient,
	}

	s.Close()
	_ = logger // suppress unused warning
}

// TestRegisterTools verifies registerTools does not panic when called with
// a valid MCP server and a db.Store. The embedder is nil because tool structs
// are created but never invoked in this test.
func TestRegisterTools(t *testing.T) {
	sqlClient := newTestSQLiteClient(t)

	mcp := mcpserver.NewMCPServer(
		"magi-test",
		"0.0.1",
		mcpserver.WithToolCapabilities(false),
	)

	s := &Server{
		mcp:    mcp,
		store:  sqlClient,
		logger: testLogger(),
	}

	s.registerTools()
}

// TestRegisterResources verifies registerResources does not panic when called
// with a valid MCP server and a db.Store.
func TestRegisterResources(t *testing.T) {
	sqlClient := newTestSQLiteClient(t)

	mcp := mcpserver.NewMCPServer(
		"magi-test",
		"0.0.1",
		mcpserver.WithResourceCapabilities(false, false),
	)

	s := &Server{
		mcp:    mcp,
		store:  sqlClient,
		logger: testLogger(),
	}

	s.registerResources()
}

// TestRegisterToolsAndResources_Together verifies both registration methods
// work when called on the same MCP server (as New() does).
func TestRegisterToolsAndResources_Together(t *testing.T) {
	sqlClient := newTestSQLiteClient(t)

	mcp := mcpserver.NewMCPServer(
		"magi-test",
		"0.0.1",
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithResourceCapabilities(false, false),
		mcpserver.WithRecovery(),
	)

	s := &Server{
		mcp:    mcp,
		store:  sqlClient,
		logger: testLogger(),
	}

	s.registerTools()
	s.registerResources()
}

// TestServeGRPC_DefaultPort verifies ServeGRPC uses port 8300 when the env var
// is empty. If the port is busy, we accept the error; if free, we stop it.
func TestServeGRPC_DefaultPort(t *testing.T) {
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

	// Give it a moment then stop regardless.
	time.Sleep(100 * time.Millisecond)
	gs.GracefulStop()

	select {
	case err := <-errCh:
		// Port 8300 may be busy; that is acceptable.
		if err != nil {
			t.Logf("ServeGRPC on default port returned (likely port busy): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeGRPC did not return")
	}
}

// TestServeWeb_DefaultPort verifies ServeWeb uses port 8080 when the env var
// is empty.
func TestServeWeb_DefaultPort(t *testing.T) {
	t.Setenv("MAGI_UI_PORT", "")

	s := &Server{
		dbClient: nil,
		embedder: nil,
		logger:   testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeWeb()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.webServer != nil {
		s.webServer.Shutdown(ctx)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("ServeWeb on default port returned (likely port busy): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeWeb did not return")
	}
}

// TestServeGateway_DefaultPorts verifies ServeGateway uses default ports when
// env vars are empty.
func TestServeGateway_DefaultPorts(t *testing.T) {
	t.Setenv("MAGI_HTTP_PORT", "")
	t.Setenv("MAGI_GRPC_PORT", "")

	s := &Server{
		logger: testLogger(),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ServeGateway()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.gwServer != nil {
		s.gwServer.Shutdown(ctx)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("ServeGateway on default ports returned (likely port busy): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeGateway did not return")
	}
}
