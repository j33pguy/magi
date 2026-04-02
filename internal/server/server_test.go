package server

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"

	"github.com/j33pguy/magi/internal/api"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	localnode "github.com/j33pguy/magi/internal/node/local"
)

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
	var _ db.Store = s.dbClient
	var _ embeddings.Provider = s.embedder
	var _ *slog.Logger = s.logger
	var _ *localnode.Coordinator = s.coordinator
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
