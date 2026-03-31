// Package grpc implements the MemoryService gRPC server.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/remember"
	"github.com/j33pguy/magi/internal/search"
	"github.com/j33pguy/magi/internal/tools"
	"github.com/j33pguy/magi/internal/vcs"
	pb "github.com/j33pguy/magi/proto/memory/v1"
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
	return &pb.HealthResponse{Ok: true, Version: "0.3.0"}, nil
}

func (s *Server) Remember(ctx context.Context, req *pb.RememberRequest) (*pb.RememberResponse, error) {
	if req.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "content is required")
	}

	source := req.Source
	if source == "" {
		source = "grpc"
	}
	input := remember.Input{
		Content:    req.Content,
		Summary:    req.Summary,
		Project:    req.Project,
		Type:       req.Type,
		Visibility: req.Visibility,
		Source:     source,
		Speaker:    req.Speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		Tags:       req.Tags,
	}
	result, err := remember.Remember(ctx, s.db, s.embedder, input, remember.Options{
		TagMode: remember.TagModeWarn,
		Logger:  s.logger,
	})
	if err != nil {
		var secretErr *remember.SecretError
		if errors.As(err, &secretErr) {
			return nil, status.Error(codes.InvalidArgument, secretErr.Error())
		}
		s.logger.Error("remember failed", "error", err)
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if result.Deduplicated {
		return &pb.RememberResponse{Id: result.Match.Memory.ID, Ok: true}, nil
	}

	resp := &pb.RememberResponse{Id: result.Saved.ID, Ok: true}
	if result.TagWarning != "" {
		tagErr := result.TagWarning
		if len(tagErr) > 80 {
			tagErr = tagErr[:80]
		}
		resp.TagWarning = "tags may not have been saved: " + tagErr
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

func (s *Server) LinkMemories(ctx context.Context, req *pb.LinkMemoriesRequest) (*pb.LinkMemoriesResponse, error) {
	if req.FromId == "" {
		return nil, status.Error(codes.InvalidArgument, "from_id is required")
	}
	if req.ToId == "" {
		return nil, status.Error(codes.InvalidArgument, "to_id is required")
	}
	if req.Relation == "" {
		return nil, status.Error(codes.InvalidArgument, "relation is required")
	}

	weight := req.Weight
	if weight == 0 {
		weight = 1.0
	}

	if _, err := s.db.GetMemory(req.FromId); err != nil {
		return nil, status.Errorf(codes.NotFound, "source memory not found: %v", err)
	}
	if _, err := s.db.GetMemory(req.ToId); err != nil {
		return nil, status.Errorf(codes.NotFound, "target memory not found: %v", err)
	}

	link, err := s.db.CreateLink(ctx, req.FromId, req.ToId, req.Relation, weight, false)
	if err != nil {
		s.logger.Error("creating link", "error", err, "from_id", req.FromId, "to_id", req.ToId)
		return nil, status.Errorf(codes.Internal, "creating link: %v", err)
	}

	return &pb.LinkMemoriesResponse{Link: memoryLinkToProto(link)}, nil
}

func (s *Server) GetRelated(ctx context.Context, req *pb.GetRelatedRequest) (*pb.GetRelatedResponse, error) {
	if req.MemoryId == "" {
		return nil, status.Error(codes.InvalidArgument, "memory_id is required")
	}

	depth := int(req.Depth)
	if depth <= 0 {
		depth = 1
	}

	direction := req.Direction
	if direction == "" {
		direction = "both"
	}

	var memoryIDs []string
	var allLinks []*db.MemoryLink

	if depth == 1 {
		links, err := s.db.GetLinks(ctx, req.MemoryId, direction)
		if err != nil {
			s.logger.Error("getting links", "error", err, "memory_id", req.MemoryId)
			return nil, status.Errorf(codes.Internal, "getting links: %v", err)
		}
		allLinks = links
		seen := map[string]bool{}
		for _, l := range links {
			neighborID := l.ToID
			if neighborID == req.MemoryId {
				neighborID = l.FromID
			}
			if !seen[neighborID] {
				seen[neighborID] = true
				memoryIDs = append(memoryIDs, neighborID)
			}
		}
	} else {
		ids, err := s.db.TraverseGraph(ctx, req.MemoryId, depth)
		if err != nil {
			s.logger.Error("traversing graph", "error", err, "memory_id", req.MemoryId, "depth", depth)
			return nil, status.Errorf(codes.Internal, "traversing graph: %v", err)
		}
		memoryIDs = ids
		links, err := s.db.GetLinks(ctx, req.MemoryId, "both")
		if err != nil {
			s.logger.Error("getting links", "error", err, "memory_id", req.MemoryId)
			return nil, status.Errorf(codes.Internal, "getting links: %v", err)
		}
		allLinks = links
	}

	var memories []*db.Memory
	for _, id := range memoryIDs {
		mem, err := s.db.GetMemory(id)
		if err != nil {
			continue
		}
		memories = append(memories, mem)
	}

	return &pb.GetRelatedResponse{
		Memories: memoriesToProto(memories),
		Links:    memoryLinksToProto(allLinks),
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

func memoryLinkToProto(link *db.MemoryLink) *pb.MemoryLink {
	if link == nil {
		return nil
	}
	return &pb.MemoryLink{
		Id:        link.ID,
		FromId:    link.FromID,
		ToId:      link.ToID,
		Relation:  link.Relation,
		Weight:    link.Weight,
		Auto:      link.Auto,
		CreatedAt: link.CreatedAt,
	}
}

func memoryLinksToProto(links []*db.MemoryLink) []*pb.MemoryLink {
	out := make([]*pb.MemoryLink, len(links))
	for i, link := range links {
		out[i] = memoryLinkToProto(link)
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
