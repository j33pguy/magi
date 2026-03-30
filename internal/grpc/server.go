// Package grpc implements the MemoryService gRPC server.
package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/vcs"
	pb "github.com/j33pguy/magi/proto/memory/v1"
	"github.com/j33pguy/magi/internal/search"
	"github.com/j33pguy/magi/internal/tools"
)

// Server implements the MemoryService gRPC service.
type Server struct {
	pb.UnimplementedMemoryServiceServer
	db       db.Store
	embedder embeddings.Provider
	logger   *slog.Logger
	gitRepo  *vcs.Repo // optional — nil if git versioning is disabled
}

// NewServer creates a new gRPC MemoryService server.
func NewServer(dbClient db.Store, embedder embeddings.Provider, logger *slog.Logger) *Server {
	return &Server{
		db:       dbClient,
		embedder: embedder,
		logger:   logger,
	}
}

func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Ok: true, Version: "0.1.0"}, nil
}

func (s *Server) Remember(ctx context.Context, req *pb.RememberRequest) (*pb.RememberResponse, error) {
	if req.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "content is required")
	}

	memType := req.Type
	if memType == "" {
		memType = "memory"
	}
	source := req.Source
	if source == "" {
		source = "grpc"
	}

	embedding, err := s.embedder.Embed(ctx, req.Content)
	if err != nil {
		s.logger.Error("generating embedding", "error", err)
		return nil, status.Errorf(codes.Internal, "generating embedding: %v", err)
	}

	speaker := req.Speaker
	if speaker == "" {
		speaker = "assistant"
	}

	memory := &db.Memory{
		Content:    req.Content,
		Summary:    req.Summary,
		Embedding:  embedding,
		Project:    req.Project,
		Type:       memType,
		Visibility: req.Visibility,
		Source:     source,
		Speaker:    speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		TokenCount: len(req.Content) / 4,
	}

	saved, err := s.db.SaveMemory(memory)
	if err != nil {
		s.logger.Error("saving memory", "error", err)
		return nil, status.Errorf(codes.Internal, "saving memory: %v", err)
	}

	// Build tags list: user-provided + structured taxonomy tags
	tags := append([]string{}, req.Tags...)
	if speaker != "" {
		tags = append(tags, "speaker:"+speaker)
	}
	if req.Area != "" {
		tags = append(tags, "area:"+req.Area)
	}
	if req.SubArea != "" {
		tags = append(tags, "sub_area:"+req.SubArea)
	}

	resp := &pb.RememberResponse{Id: saved.ID, Ok: true}
	if len(tags) > 0 {
		if err := s.db.SetTags(saved.ID, tags); err != nil {
			s.logger.Warn("setting tags failed (non-fatal)", "error", err, "memory_id", saved.ID)
			tagErr := err.Error()
			if len(tagErr) > 80 {
				tagErr = tagErr[:80]
			}
			resp.TagWarning = "tags may not have been saved: " + tagErr
		}
	}

	return resp, nil
}

func (s *Server) Recall(ctx context.Context, req *pb.RecallRequest) (*pb.RecallResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	topK := int(req.TopK)
	if topK <= 0 {
		topK = 5
	}

	afterTime, err := tools.ParseTimeParam(req.AfterTime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid after_time: %v", err)
	}
	beforeTime, err := tools.ParseTimeParam(req.BeforeTime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid before_time: %v", err)
	}

	filter := &db.MemoryFilter{
		Project:    req.Project,
		Projects:   req.Projects,
		Type:       req.Type,
		Tags:       req.Tags,
		Visibility: "", // gRPC API: exclude private memories by default
		Speaker:    req.Speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		AfterTime:  afterTime,
		BeforeTime: beforeTime,
	}

	resp, err := search.Adaptive(ctx, s.db, s.embedder.Embed, req.Query, filter, topK, req.MinRelevance, req.RecencyDecay)
	if err != nil {
		s.logger.Error("adaptive search", "error", err)
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}

	return &pb.RecallResponse{
		Results:        hybridResultsToProto(resp.Results),
		Rewritten:      resp.Rewritten,
		RewrittenQuery: resp.RewrittenQuery,
		Attempts:       int32(resp.Attempts),
	}, nil
}

func (s *Server) Forget(_ context.Context, req *pb.ForgetRequest) (*pb.ForgetResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if _, err := s.db.GetMemory(req.Id); err != nil {
		return nil, status.Errorf(codes.NotFound, "memory not found: %v", err)
	}

	if err := s.db.ArchiveMemory(req.Id); err != nil {
		s.logger.Error("archiving memory", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "archiving memory: %v", err)
	}

	return &pb.ForgetResponse{Id: req.Id, Ok: true}, nil
}

func (s *Server) List(_ context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	var tags []string
	if req.Tags != "" {
		tags = strings.Split(req.Tags, ",")
	}

	afterTime, err := tools.ParseTimeParam(req.AfterTime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid after_time: %v", err)
	}
	beforeTime, err := tools.ParseTimeParam(req.BeforeTime)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid before_time: %v", err)
	}

	filter := &db.MemoryFilter{
		Project:    req.Project,
		Type:       req.Type,
		Tags:       tags,
		Limit:      limit,
		Offset:     int(req.Offset),
		Visibility: "", // exclude private by default
		Speaker:    req.Speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		AfterTime:  afterTime,
		BeforeTime: beforeTime,
	}

	memories, err := s.db.ListMemories(filter)
	if err != nil {
		s.logger.Error("listing memories", "error", err)
		return nil, status.Errorf(codes.Internal, "listing memories: %v", err)
	}

	for _, m := range memories {
		t, err := s.db.GetTags(m.ID)
		if err != nil {
			s.logger.Error("getting tags", "error", err, "memory_id", m.ID)
			continue
		}
		m.Tags = t
	}

	return &pb.ListResponse{Memories: memoriesToProto(memories)}, nil
}

func (s *Server) CreateConversation(ctx context.Context, req *pb.CreateConversationRequest) (*pb.CreateConversationResponse, error) {
	if req.Summary == "" {
		return nil, status.Error(codes.InvalidArgument, "summary is required")
	}
	if req.Channel == "" {
		return nil, status.Error(codes.InvalidArgument, "channel is required")
	}

	content := formatConversationContent(req)

	embedding, err := s.embedder.Embed(ctx, content)
	if err != nil {
		s.logger.Error("generating embedding", "error", err)
		return nil, status.Errorf(codes.Internal, "generating embedding: %v", err)
	}

	memory := &db.Memory{
		Content:    content,
		Summary:    req.Summary,
		Embedding:  embedding,
		Type:       "conversation",
		Visibility: "private",
		Source:     req.Channel,
		TokenCount: len(content) / 4,
	}

	saved, err := s.db.SaveMemory(memory)
	if err != nil {
		s.logger.Error("saving conversation", "error", err)
		return nil, status.Errorf(codes.Internal, "saving conversation: %v", err)
	}

	tags := []string{"channel:" + req.Channel, "conversation"}
	for _, topic := range req.Topics {
		tags = append(tags, "topic:"+topic)
	}

	resp := &pb.CreateConversationResponse{Id: saved.ID, Ok: true}
	if err := s.db.SetTags(saved.ID, tags); err != nil {
		s.logger.Warn("setting conversation tags failed (non-fatal)", "error", err, "memory_id", saved.ID)
		tagErr := err.Error()
		if len(tagErr) > 80 {
			tagErr = tagErr[:80]
		}
		resp.TagWarning = "tags may not have been saved: " + tagErr
	}

	return resp, nil
}

func (s *Server) SearchConversations(ctx context.Context, req *pb.SearchConversationsRequest) (*pb.SearchConversationsResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 5
	}

	var tags []string
	if req.Channel != "" {
		tags = append(tags, "channel:"+req.Channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Visibility: "all",
	}

	resp, err := search.Adaptive(ctx, s.db, s.embedder.Embed, req.Query, filter, limit, req.MinRelevance, req.RecencyDecay)
	if err != nil {
		s.logger.Error("searching conversations", "error", err)
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}

	return &pb.SearchConversationsResponse{
		Results:        hybridResultsToProto(resp.Results),
		Rewritten:      resp.Rewritten,
		RewrittenQuery: resp.RewrittenQuery,
		Attempts:       int32(resp.Attempts),
	}, nil
}

// Conversion helpers

func memoryToProto(m *db.Memory) *pb.Memory {
	return &pb.Memory{
		Id:         m.ID,
		Content:    m.Content,
		Summary:    m.Summary,
		Project:    m.Project,
		Type:       m.Type,
		Visibility: m.Visibility,
		Source:     m.Source,
		SourceFile: m.SourceFile,
		ParentId:   m.ParentID,
		ChunkIndex: int32(m.ChunkIndex),
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
		TokenCount: int32(m.TokenCount),
		Tags:       m.Tags,
		Speaker:    m.Speaker,
		Area:       m.Area,
		SubArea:    m.SubArea,
	}
}

func memoriesToProto(memories []*db.Memory) []*pb.Memory {
	out := make([]*pb.Memory, len(memories))
	for i, m := range memories {
		out[i] = memoryToProto(m)
	}
	return out
}

func hybridResultsToProto(results []*db.HybridResult) []*pb.MemoryResult {
	out := make([]*pb.MemoryResult, len(results))
	for i, r := range results {
		out[i] = &pb.MemoryResult{
			Memory:        memoryToProto(r.Memory),
			RrfScore:      r.RRFScore,
			VecRank:       int32(r.VecRank),
			Bm25Rank:      int32(r.BM25Rank),
			Distance:      r.Distance,
			Score:         r.Score,
			RecencyWeight: r.RecencyWeight,
			WeightedScore: r.WeightedScore,
		}
	}
	return out
}

func formatConversationContent(req *pb.CreateConversationRequest) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Conversation on %s", req.Channel))
	if req.SessionKey != "" {
		b.WriteString(fmt.Sprintf(" (session: %s)", req.SessionKey))
	}
	b.WriteString("\n")

	if req.StartedAt != "" || req.EndedAt != "" {
		b.WriteString(fmt.Sprintf("Time: %s to %s\n", req.StartedAt, req.EndedAt))
	}
	if req.TurnCount > 0 {
		b.WriteString(fmt.Sprintf("Turns: %d\n", req.TurnCount))
	}

	if len(req.Topics) > 0 {
		b.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(req.Topics, ", ")))
	}

	b.WriteString("\n")
	b.WriteString(req.Summary)

	if len(req.Decisions) > 0 {
		b.WriteString("\n\nDecisions:\n")
		for _, d := range req.Decisions {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
	}

	if len(req.ActionItems) > 0 {
		b.WriteString("\nAction Items:\n")
		for _, a := range req.ActionItems {
			b.WriteString(fmt.Sprintf("- %s\n", a))
		}
	}

	return b.String()
}

