// Package server wires together the MCP server with tools, resources, and database.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/j33pguy/claude-memory/api"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
	memgrpc "github.com/j33pguy/claude-memory/grpc"
	pb "github.com/j33pguy/claude-memory/proto/memory/v1"
	"github.com/j33pguy/claude-memory/resources"
	"github.com/j33pguy/claude-memory/tools"
	"github.com/j33pguy/claude-memory/web"
)

// Server is the claude-memory MCP server.
type Server struct {
	mcp        *mcpserver.MCPServer
	httpAPI    *api.Server
	grpcServer *grpc.Server
	gwServer   *http.Server
	webServer  *http.Server
	dbClient   *db.Client
	embedder   *embeddings.OnnxProvider
	logger     *slog.Logger
}

// New creates and configures a new claude-memory MCP server.
func New(logger *slog.Logger) (*Server, error) {
	// Initialize database
	dbCfg := db.ConfigFromEnv()
	dbClient, err := db.NewClient(dbCfg, logger.WithGroup("db"))
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Run migrations
	if err := dbClient.Migrate(); err != nil {
		dbClient.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	// Initialize embedding provider
	embedder, err := embeddings.NewOnnxProvider(logger.WithGroup("embeddings"))
	if err != nil {
		dbClient.Close()
		return nil, fmt.Errorf("initializing embeddings: %w", err)
	}

	s := &Server{
		dbClient: dbClient,
		embedder: embedder,
		logger:   logger,
	}

	s.mcp = mcpserver.NewMCPServer(
		"claude-memory",
		"0.1.0",
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithResourceCapabilities(false, false),
		mcpserver.WithRecovery(),
	)

	s.registerTools()
	s.registerResources()

	// gRPC server with auth interceptor
	token := os.Getenv("CLAUDE_MEMORY_API_TOKEN")
	s.grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(memgrpc.AuthInterceptor(token)),
	)
	grpcSvc := memgrpc.NewServer(s.dbClient, s.embedder, logger.WithGroup("grpc"))
	pb.RegisterMemoryServiceServer(s.grpcServer, grpcSvc)

	// Keep existing HTTP API (will be deprecated once grpc-gateway is proven)
	s.httpAPI = api.NewServer(s.dbClient, s.embedder, logger.WithGroup("http"))

	return s, nil
}

func (s *Server) registerTools() {
	remember := &tools.Remember{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(remember.Tool(), remember.Handle)

	recall := &tools.Recall{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(recall.Tool(), recall.Handle)

	recallIncidents := &tools.RecallIncidents{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(recallIncidents.Tool(), recallIncidents.Handle)

	recallLessons := &tools.RecallLessons{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(recallLessons.Tool(), recallLessons.Handle)

	forget := &tools.Forget{DB: s.dbClient}
	s.mcp.AddTool(forget.Tool(), forget.Handle)

	list := &tools.List{DB: s.dbClient}
	s.mcp.AddTool(list.Tool(), list.Handle)

	update := &tools.Update{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(update.Tool(), update.Handle)

	storeConv := &tools.StoreConversation{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(storeConv.Tool(), storeConv.Handle)

	recallConv := &tools.RecallConversations{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(recallConv.Tool(), recallConv.Handle)

	recentConv := &tools.RecentConversations{DB: s.dbClient}
	s.mcp.AddTool(recentConv.Tool(), recentConv.Handle)

	indexTurn := &tools.IndexTurn{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(indexTurn.Tool(), indexTurn.Handle)

	indexSession := &tools.IndexSession{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(indexSession.Tool(), indexSession.Handle)

	checkContra := &tools.CheckContradictions{DB: s.dbClient, Embedder: s.embedder}
	s.mcp.AddTool(checkContra.Tool(), checkContra.Handle)
linkMemories := &tools.LinkMemories{DB: s.dbClient}
	s.mcp.AddTool(linkMemories.Tool(), linkMemories.Handle)

	getRelated := &tools.GetRelated{DB: s.dbClient}
	s.mcp.AddTool(getRelated.Tool(), getRelated.Handle)

	unlinkMemories := &tools.UnlinkMemories{DB: s.dbClient}
	s.mcp.AddTool(unlinkMemories.Tool(), unlinkMemories.Handle)
}

func (s *Server) registerResources() {
	recent := &resources.Recent{DB: s.dbClient}
	s.mcp.AddResourceTemplate(recent.Template(), recent.Handle)

	decisions := &resources.Decisions{DB: s.dbClient}
	s.mcp.AddResourceTemplate(decisions.Template(), decisions.Handle)

	prefs := &resources.Preferences{DB: s.dbClient}
	s.mcp.AddResource(prefs.Resource(), prefs.Handle)

	ctx := &resources.Context{DB: s.dbClient}
	s.mcp.AddResource(ctx.Resource(), ctx.Handle)

	recentConv := &resources.RecentConversations{DB: s.dbClient}
	s.mcp.AddResource(recentConv.Resource(), recentConv.Handle)
pats := &resources.Patterns{DB: s.dbClient}
	s.mcp.AddResource(pats.Resource(), pats.Handle)
}

// ServeGRPC starts the gRPC server. Blocks until the server stops.
func (s *Server) ServeGRPC() error {
	port := os.Getenv("CLAUDE_MEMORY_GRPC_PORT")
	if port == "" {
		port = "8300"
	}

	lis, err := net.Listen("tcp", net.JoinHostPort("", port))
	if err != nil {
		return fmt.Errorf("gRPC listen: %w", err)
	}

	s.logger.Info("Starting gRPC server", "addr", lis.Addr().String())
	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("gRPC serve: %w", err)
	}
	return nil
}

// ServeGateway starts the grpc-gateway HTTP/JSON reverse proxy.
// It connects to the gRPC server and proxies JSON requests.
func (s *Server) ServeGateway() error {
	port := os.Getenv("CLAUDE_MEMORY_HTTP_PORT")
	if port == "" {
		port = "8301"
	}
	grpcPort := os.Getenv("CLAUDE_MEMORY_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "8300"
	}

	ctx := context.Background()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	grpcAddr := net.JoinHostPort("localhost", grpcPort)
	if err := pb.RegisterMemoryServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts); err != nil {
		return fmt.Errorf("registering gateway: %w", err)
	}

	s.gwServer = &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.logger.Info("Starting grpc-gateway HTTP server", "addr", s.gwServer.Addr, "upstream", grpcAddr)
	if err := s.gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("gateway http: %w", err)
	}
	return nil
}

// ServeWeb starts the web UI server. Blocks until the server stops.
func (s *Server) ServeWeb() error {
	port := os.Getenv("CLAUDE_MEMORY_UI_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	web.RegisterRoutes(mux, s.dbClient, s.embedder, s.logger.WithGroup("web"))

	s.webServer = &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.logger.Info("Starting web UI server", "addr", s.webServer.Addr)
	if err := s.webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web UI server: %w", err)
	}
	return nil
}

// ShutdownWeb gracefully stops the web UI server.
func (s *Server) ShutdownWeb(ctx context.Context) error {
	if s.webServer != nil {
		return s.webServer.Shutdown(ctx)
	}
	return nil
}

// ServeHTTP starts the legacy HTTP API server. Blocks until the server stops.
func (s *Server) ServeHTTP() error {
	return s.httpAPI.Start()
}

// ShutdownHTTP gracefully stops the legacy HTTP API server.
func (s *Server) ShutdownHTTP(ctx context.Context) error {
	return s.httpAPI.Shutdown(ctx)
}

// ShutdownGRPC gracefully stops the gRPC server and gateway.
func (s *Server) ShutdownGRPC(ctx context.Context) error {
	s.grpcServer.GracefulStop()
	if s.gwServer != nil {
		return s.gwServer.Shutdown(ctx)
	}
	return nil
}

// Run starts the MCP server on stdio.
func (s *Server) Run() error {
	s.logger.Info("Starting claude-memory MCP server")
	return mcpserver.ServeStdio(s.mcp)
}

// Close shuts down the server, cleaning up database connections and ONNX runtime.
func (s *Server) Close() {
	if s.embedder != nil {
		s.embedder.Destroy()
	}
	if s.dbClient != nil {
		s.dbClient.Close()
	}
}
