// Package server wires together the MCP server with tools, resources, and database.
package server

import (
	"context"
	"fmt"
	"log/slog"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/j33pguy/claude-memory/api"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
	"github.com/j33pguy/claude-memory/resources"
	"github.com/j33pguy/claude-memory/tools"
)

// Server is the claude-memory MCP server.
type Server struct {
	mcp      *mcpserver.MCPServer
	httpAPI  *api.Server
	dbClient *db.Client
	embedder *embeddings.OnnxProvider
	logger   *slog.Logger
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
}

func (s *Server) registerResources() {
	recent := &resources.Recent{DB: s.dbClient}
	s.mcp.AddResourceTemplate(recent.Template(), recent.Handle)

	decisions := &resources.Decisions{DB: s.dbClient}
	s.mcp.AddResourceTemplate(decisions.Template(), decisions.Handle)

	prefs := &resources.Preferences{DB: s.dbClient}
	s.mcp.AddResource(prefs.Resource(), prefs.Handle)
}

// ServeHTTP starts the HTTP API server. Blocks until the server stops.
func (s *Server) ServeHTTP() error {
	return s.httpAPI.Start()
}

// ShutdownHTTP gracefully stops the HTTP API server.
func (s *Server) ShutdownHTTP(ctx context.Context) error {
	return s.httpAPI.Shutdown(ctx)
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
