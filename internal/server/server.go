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
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/j33pguy/magi/internal/api"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	memgrpc "github.com/j33pguy/magi/internal/grpc"
	"github.com/j33pguy/magi/internal/node"
	localnode "github.com/j33pguy/magi/internal/node/local"
	"github.com/j33pguy/magi/internal/project"
	"github.com/j33pguy/magi/internal/resources"
	"github.com/j33pguy/magi/internal/syncstate"
	"github.com/j33pguy/magi/internal/tools"
	"github.com/j33pguy/magi/internal/vcs"
	"github.com/j33pguy/magi/internal/web"
	pb "github.com/j33pguy/magi/proto/memory/v1"
)

// Server is the magi MCP server.
type Server struct {
	mcp         *mcpserver.MCPServer
	httpAPI     *api.Server
	grpcServer  *grpc.Server
	gwServer    *http.Server
	webServer   *http.Server
	dbClient    db.Store
	store       db.Store // either dbClient directly, or a VersionedStore wrapper
	embedder    *embeddings.OnnxProvider
	logger      *slog.Logger
	gitRepo     *vcs.Repo              // nil if git versioning is disabled
	coordinator *localnode.Coordinator // nil if coordinator is disabled
	project     string
	syncTracker *syncstate.Tracker
}

// New creates and configures a new magi MCP server.
func New(logger *slog.Logger) (*Server, error) {
	// Initialize database
	dbCfg := db.ConfigFromEnv()
	dbClient, err := db.NewStore(dbCfg, logger.WithGroup("db"))
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
		dbClient:    dbClient,
		store:       dbClient, // default: use raw client
		embedder:    embedder,
		logger:      logger,
		syncTracker: syncstate.NewTracker(),
	}

	cwd, err := os.Getwd()
	if err != nil {
		logger.Warn("failed to resolve working directory for project detection", "error", err)
		cwd = "."
	}
	s.project = project.DetectProject(cwd)
	if s.project != "" {
		logger.Info("Detected project", "project", s.project)
	}

	// Git versioning (optional)
	gitCfg := vcs.ConfigFromEnv()
	if gitCfg.Enabled {
		gitRepo, err := vcs.Init(gitCfg)
		if err != nil {
			logger.Error("git versioning init failed, continuing without git", "error", err)
		} else {
			s.gitRepo = gitRepo
			logger.Info("Git versioning enabled", "path", gitCfg.Path, "mode", gitCfg.CommitMode)

			// Rebuild DB from git if DB is empty but git has memories
			if vcs.DBIsEmpty(dbClient) && gitRepo.HasMemories() {
				logger.Info("DB is empty but git repo has memories, rebuilding...")
				if err := vcs.RebuildDB(dbClient, gitRepo, embedder, logger.WithGroup("rebuild")); err != nil {
					logger.Error("git rebuild failed", "error", err)
				}
			}

			// Wrap with versioned store — intercepts mutations to also write to git.
			// Requires concrete *Client (turso/sqlite only); SQL Server skips VCS wrapping.
			if concreteClient, ok := dbClient.(*db.Client); ok {
				s.store = vcs.NewVersionedStore(concreteClient, gitRepo, logger.WithGroup("vcs"))
			} else {
				logger.Warn("git versioning not supported for this backend, using raw store")
			}
		}
	}

	// Node mesh coordinator (Phase 1: embedded mode)
	nodeCfg := node.ConfigFromEnv()
	if nodeCfg.CoordinatorEnabled {
		coord := localnode.NewCoordinator(nodeCfg, s.store, logger.WithGroup("node"))
		if err := coord.Start(context.Background()); err != nil {
			s.Close()
			return nil, fmt.Errorf("starting node coordinator: %w", err)
		}
		s.coordinator = coord
		// Wrap store so all tools/gRPC/API route through the node pools.
		s.store = localnode.NewCoordinatedStore(coord, s.store)
	}

	s.mcp = mcpserver.NewMCPServer(
		"magi",
		"0.1.0",
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithResourceCapabilities(false, false),
		mcpserver.WithRecovery(),
	)

	s.registerTools()
	s.registerResources()

	// gRPC server with auth interceptor
	token := os.Getenv("MAGI_API_TOKEN")
	s.grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(memgrpc.AuthInterceptor(token)),
	)
	grpcSvc := memgrpc.NewServer(s.store, s.embedder, logger.WithGroup("grpc"))
	if s.gitRepo != nil {
		grpcSvc.SetGitRepo(s.gitRepo)
	}
	pb.RegisterMemoryServiceServer(s.grpcServer, grpcSvc)

	// Keep existing HTTP API (will be deprecated once grpc-gateway is proven)
	s.httpAPI = api.NewServer(s.store, s.embedder, logger.WithGroup("http"))
	if s.gitRepo != nil {
		s.httpAPI.SetGitRepo(s.gitRepo)
	}

	return s, nil
}

func (s *Server) registerTools() {
	remember := &tools.Remember{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(remember.Tool(), remember.Handle)

	recall := &tools.Recall{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(recall.Tool(), recall.Handle)

	recallIncidents := &tools.RecallIncidents{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(recallIncidents.Tool(), recallIncidents.Handle)

	recallLessons := &tools.RecallLessons{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(recallLessons.Tool(), recallLessons.Handle)

	forget := &tools.Forget{DB: s.store}
	s.addTool(forget.Tool(), forget.Handle)

	list := &tools.List{DB: s.store, DefaultProject: s.project}
	s.addTool(list.Tool(), list.Handle)

	update := &tools.Update{DB: s.store, Embedder: s.embedder}
	s.addTool(update.Tool(), update.Handle)

	storeConv := &tools.StoreConversation{DB: s.store, Embedder: s.embedder}
	s.addTool(storeConv.Tool(), storeConv.Handle)

	recallConv := &tools.RecallConversations{DB: s.store, Embedder: s.embedder}
	s.addTool(recallConv.Tool(), recallConv.Handle)

	recentConv := &tools.RecentConversations{DB: s.store}
	s.addTool(recentConv.Tool(), recentConv.Handle)

	indexTurn := &tools.IndexTurn{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(indexTurn.Tool(), indexTurn.Handle)

	indexSession := &tools.IndexSession{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(indexSession.Tool(), indexSession.Handle)

	checkContra := &tools.CheckContradictions{DB: s.store, Embedder: s.embedder}
	s.addTool(checkContra.Tool(), checkContra.Handle)

	linkMemories := &tools.LinkMemories{DB: s.store}
	s.addTool(linkMemories.Tool(), linkMemories.Handle)

	getRelated := &tools.GetRelated{DB: s.store}
	s.addTool(getRelated.Tool(), getRelated.Handle)

	unlinkMemories := &tools.UnlinkMemories{DB: s.store}
	s.addTool(unlinkMemories.Tool(), unlinkMemories.Handle)

	ingestConv := &tools.IngestConversation{DB: s.store, Embedder: s.embedder, DefaultProject: s.project}
	s.addTool(ingestConv.Tool(), ingestConv.Handle)

	syncNow := &tools.SyncNow{DB: s.store, Project: s.project, Tracker: s.syncTracker}
	s.addTool(syncNow.Tool(), syncNow.Handle)
}

func (s *Server) registerResources() {
	recent := &resources.Recent{DB: s.store}
	s.mcp.AddResourceTemplate(recent.Template(), recent.Handle)

	decisions := &resources.Decisions{DB: s.store}
	s.mcp.AddResourceTemplate(decisions.Template(), decisions.Handle)

	prefs := &resources.Preferences{DB: s.store}
	s.mcp.AddResource(prefs.Resource(), prefs.Handle)

	ctx := &resources.Context{DB: s.store, DefaultProject: s.project}
	s.mcp.AddResource(ctx.Resource(), ctx.Handle)

	recentConv := &resources.RecentConversations{DB: s.store}
	s.mcp.AddResource(recentConv.Resource(), recentConv.Handle)

	pats := &resources.Patterns{DB: s.store}
	s.mcp.AddResource(pats.Resource(), pats.Handle)

	syncStatus := &resources.SyncStatus{DB: s.store, Project: s.project, Tracker: s.syncTracker}
	s.mcp.AddResource(syncStatus.Resource(), syncStatus.Handle)
}

func (s *Server) addTool(tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	s.mcp.AddTool(tool, s.withSyncGate(handler))
}

func (s *Server) withSyncGate(handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureFreshSync(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
		}
		return handler(ctx, request)
	}
}

func (s *Server) ensureFreshSync() error {
	if s.syncTracker == nil {
		return nil
	}

	syncer, ok := s.store.(interface{ Sync() error })
	if !ok {
		return nil
	}

	snap := s.syncTracker.Snapshot()
	if snap.Syncing {
		s.syncTracker.Wait()
		snap = s.syncTracker.Snapshot()
	}

	maxAge := syncstate.MaxAge()
	if !snap.LastSync.IsZero() && time.Since(snap.LastSync) <= maxAge {
		return nil
	}

	if !s.syncTracker.Start() {
		s.syncTracker.Wait()
		return nil
	}

	s.logger.Info("syncing project memories", "project", s.project)
	err := syncer.Sync()
	s.syncTracker.Finish(err)
	if err != nil {
		return err
	}
	return nil
}

// ServeGRPC starts the gRPC server. Blocks until the server stops.
func (s *Server) ServeGRPC() error {
	port := os.Getenv("MAGI_GRPC_PORT")
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
	port := os.Getenv("MAGI_HTTP_PORT")
	if port == "" {
		port = "8301"
	}
	grpcPort := os.Getenv("MAGI_GRPC_PORT")
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
	port := os.Getenv("MAGI_UI_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	// Web UI requires concrete *db.Client for raw stats queries.
	// SQL Server backend skips the web UI registration.
	if concreteClient, ok := s.dbClient.(*db.Client); ok {
		web.RegisterRoutes(mux, concreteClient, s.embedder, s.logger.WithGroup("web"))
	} else {
		s.logger.Warn("web UI not available for this storage backend")
	}

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
	s.logger.Info("Starting magi MCP server")
	return mcpserver.ServeStdio(s.mcp)
}

// Close shuts down the server, cleaning up database connections and ONNX runtime.
func (s *Server) Close() {
	if s.coordinator != nil {
		s.coordinator.Stop()
	}
	if s.gitRepo != nil {
		s.gitRepo.Close()
	}
	if s.embedder != nil {
		s.embedder.Destroy()
	}
	if s.dbClient != nil {
		s.dbClient.Close()
	}
}
