package grpc

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/j33pguy/magi/internal/db"
	pb "github.com/j33pguy/magi/proto/memory/v1"
)

// ---------------------------------------------------------------------------
// Mock embedder
// ---------------------------------------------------------------------------

type mockEmbedder struct {
	failNext bool
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if m.failNext {
		m.failNext = false
		return nil, errFakeEmbed
	}
	return mockVector(text), nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = mockVector(texts[i])
	}
	return out, nil
}

func (m *mockEmbedder) Dimensions() int { return 384 }

var errFakeEmbed = status.Error(codes.Internal, "fake embed failure")

func mockVector(text string) []float32 {
	emb := make([]float32, 384)
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(text))
	sum := hasher.Sum32()
	for i := 0; i < 8; i++ {
		nibble := int32((sum >> (4 * i)) & 0xF)
		emb[i] = float32(nibble-8) / 8.0
	}
	return emb
}

// ---------------------------------------------------------------------------
// Test helper: create Server with real SQLite + mock embedder
// ---------------------------------------------------------------------------

func newTestGRPCServer(t *testing.T) (*Server, *mockEmbedder) {
	t.Helper()

	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	emb := &mockEmbedder{}
	srv := NewServer(client.TursoClient, emb, logger)
	return srv, emb
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func TestHealth_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	resp, err := srv.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.Version != "0.3.0" {
		t.Errorf("version = %q, want 0.3.0", resp.Version)
	}
}

// ---------------------------------------------------------------------------
// Remember
// ---------------------------------------------------------------------------

func TestRemember_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	resp, err := srv.Remember(ctx, &pb.RememberRequest{
		Content:    "The sky is blue",
		Summary:    "sky color",
		Project:    "test-project",
		Type:       "fact",
		Visibility: "internal",
		Tags:       []string{"color", "nature"},
		Source:     "unit-test",
		Speaker:    "user",
		Area:       "science",
		SubArea:    "physics",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.Id == "" {
		t.Error("expected non-empty id")
	}
}

func TestRemember_Defaults(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	resp, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "minimal memory",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if !resp.Ok || resp.Id == "" {
		t.Error("expected ok=true with non-empty id")
	}
}

func TestRemember_EmptyContent(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.Remember(context.Background(), &pb.RememberRequest{Content: ""})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestRemember_EmbedError(t *testing.T) {
	srv, emb := newTestGRPCServer(t)
	emb.failNext = true
	_, err := srv.Remember(context.Background(), &pb.RememberRequest{Content: "test"})
	assertGRPCCode(t, err, codes.Internal)
}

func TestRemember_WithTags(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	resp, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "tagged memory",
		Tags:    []string{"important", "review"},
		Speaker: "alice",
		Area:    "work",
		SubArea: "magi",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	// Verify tags were stored by listing
	listResp, err := srv.List(ctx, &pb.ListRequest{Tags: "important"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listResp.Memories) == 0 {
		t.Error("expected at least 1 memory with tag 'important'")
	}
	found := false
	for _, m := range listResp.Memories {
		if m.Id == resp.Id {
			found = true
			break
		}
	}
	if !found {
		t.Error("remembered memory not found in tag-filtered list")
	}
}

// ---------------------------------------------------------------------------
// Recall
// ---------------------------------------------------------------------------

func TestRecall_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	// Seed a memory first
	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "Go is a statically typed language",
		Project: "test-project",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.Recall(ctx, &pb.RecallRequest{
		Query:   "statically typed",
		Project: "test-project",
		TopK:    5,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	// With mock embedder returning identical vectors, the seeded memory should appear
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRecall_EmptyQuery(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.Recall(context.Background(), &pb.RecallRequest{Query: ""})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestRecall_DefaultTopK(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	// Seed a few memories
	for i := 0; i < 3; i++ {
		_, err := srv.Remember(ctx, &pb.RememberRequest{Content: "memory for recall test"})
		if err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}

	resp, err := srv.Recall(ctx, &pb.RecallRequest{
		Query: "recall test",
		TopK:  0, // should default to 5
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRecall_InvalidAfterTime(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.Recall(context.Background(), &pb.RecallRequest{
		Query:     "test",
		AfterTime: "not-a-time",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestRecall_InvalidBeforeTime(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.Recall(context.Background(), &pb.RecallRequest{
		Query:      "test",
		BeforeTime: "not-a-time",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestRecall_WithFilters(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "filtered recall memory",
		Project: "proj-a",
		Speaker: "user",
		Area:    "work",
		SubArea: "magi",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.Recall(ctx, &pb.RecallRequest{
		Query:   "filtered recall",
		Project: "proj-a",
		Speaker: "user",
		Area:    "work",
		SubArea: "magi",
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRecall_WithRelativeTime(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{Content: "recent memory"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.Recall(ctx, &pb.RecallRequest{
		Query:     "recent",
		AfterTime: "7d",
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Forget
// ---------------------------------------------------------------------------

func TestForget_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	rem, err := srv.Remember(ctx, &pb.RememberRequest{Content: "to be forgotten"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.Forget(ctx, &pb.ForgetRequest{Id: rem.Id})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.Id != rem.Id {
		t.Errorf("id = %q, want %q", resp.Id, rem.Id)
	}
}

func TestForget_EmptyID(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.Forget(context.Background(), &pb.ForgetRequest{Id: ""})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestForget_NotFound(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.Forget(context.Background(), &pb.ForgetRequest{Id: "nonexistent-id"})
	assertGRPCCode(t, err, codes.NotFound)
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	// Seed memories
	for i := 0; i < 3; i++ {
		_, err := srv.Remember(ctx, &pb.RememberRequest{
			Content: fmt.Sprintf("list test memory %d", i),
			Project: "list-proj",
			Type:    "fact",
		})
		if err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}

	resp, err := srv.List(ctx, &pb.ListRequest{
		Project: "list-proj",
		Type:    "fact",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Memories) != 3 {
		t.Errorf("got %d memories, want 3", len(resp.Memories))
	}
}

func TestList_DefaultLimit(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{Content: "one more"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.List(ctx, &pb.ListRequest{Limit: 0}) // should default to 20
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestList_WithOffset(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := srv.Remember(ctx, &pb.RememberRequest{
			Content: fmt.Sprintf("offset test %d", i),
			Project: "offset-proj",
		})
		if err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}

	resp, err := srv.List(ctx, &pb.ListRequest{
		Project: "offset-proj",
		Limit:   2,
		Offset:  2,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Memories) != 2 {
		t.Errorf("got %d memories, want 2", len(resp.Memories))
	}
}

func TestList_WithTags(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "tagged list item",
		Tags:    []string{"listtest"},
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.List(ctx, &pb.ListRequest{Tags: "listtest"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Memories) == 0 {
		t.Error("expected at least 1 memory with tag 'listtest'")
	}
}

func TestList_WithSpeakerAreaFilters(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "filtered list item",
		Speaker: "agent",
		Area:    "infrastructure",
		SubArea: "compute",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.List(ctx, &pb.ListRequest{
		Speaker: "agent",
		Area:    "infrastructure",
		SubArea: "compute",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Memories) == 0 {
		t.Error("expected at least 1 memory matching filters")
	}
}

func TestList_InvalidAfterTime(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.List(context.Background(), &pb.ListRequest{AfterTime: "bad"})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestList_InvalidBeforeTime(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.List(context.Background(), &pb.ListRequest{BeforeTime: "bad"})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestList_EmptyResult(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	resp, err := srv.List(context.Background(), &pb.ListRequest{
		Project: "nonexistent-project",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Errorf("got %d memories, want 0", len(resp.Memories))
	}
}

// ---------------------------------------------------------------------------
// CreateConversation
// ---------------------------------------------------------------------------

func TestCreateConversation_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	resp, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel:     "slack-general",
		Summary:     "Discussed deployment strategy",
		SessionKey:  "session-123",
		StartedAt:   "2025-01-01T10:00:00Z",
		EndedAt:     "2025-01-01T11:00:00Z",
		TurnCount:   42,
		Topics:      []string{"deployment", "k8s"},
		Decisions:   []string{"Use blue-green deployment"},
		ActionItems: []string{"Write runbook for rollback"},
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.Id == "" {
		t.Error("expected non-empty id")
	}
}

func TestCreateConversation_Minimal(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	resp, err := srv.CreateConversation(context.Background(), &pb.CreateConversationRequest{
		Channel: "discord",
		Summary: "Quick chat",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if !resp.Ok || resp.Id == "" {
		t.Error("expected ok=true with non-empty id")
	}
}

func TestCreateConversation_MissingSummary(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.CreateConversation(context.Background(), &pb.CreateConversationRequest{
		Channel: "test",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestCreateConversation_MissingChannel(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.CreateConversation(context.Background(), &pb.CreateConversationRequest{
		Summary: "test",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestCreateConversation_EmbedError(t *testing.T) {
	srv, emb := newTestGRPCServer(t)
	emb.failNext = true
	_, err := srv.CreateConversation(context.Background(), &pb.CreateConversationRequest{
		Channel: "test",
		Summary: "test",
	})
	assertGRPCCode(t, err, codes.Internal)
}

// ---------------------------------------------------------------------------
// SearchConversations
// ---------------------------------------------------------------------------

func TestSearchConversations_Success(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	// Seed a conversation
	_, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "slack-general",
		Summary: "Discussed memory architecture",
		Topics:  []string{"architecture"},
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	resp, err := srv.SearchConversations(ctx, &pb.SearchConversationsRequest{
		Query:   "memory architecture",
		Limit:   5,
		Channel: "slack-general",
	})
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestSearchConversations_EmptyQuery(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	_, err := srv.SearchConversations(context.Background(), &pb.SearchConversationsRequest{Query: ""})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestSearchConversations_DefaultLimit(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "test",
		Summary: "default limit test",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	resp, err := srv.SearchConversations(ctx, &pb.SearchConversationsRequest{
		Query: "default limit",
		Limit: 0, // defaults to 5
	})
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestSearchConversations_NoChannel(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	_, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "any-channel",
		Summary: "no channel filter test",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	resp, err := srv.SearchConversations(ctx, &pb.SearchConversationsRequest{
		Query: "no channel filter",
		// no Channel filter
	})
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

func TestMemoryToProto(t *testing.T) {
	m := &db.Memory{
		ID:         "abc123",
		Content:    "test content",
		Summary:    "test summary",
		Project:    "proj",
		Type:       "fact",
		Visibility: "internal",
		Source:     "grpc",
		SourceFile: "file.go",
		ParentID:   "parent1",
		ChunkIndex: 3,
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-02T00:00:00Z",
		TokenCount: 10,
		Tags:       []string{"tag1", "tag2"},
		Speaker:    "user",
		Area:       "work",
		SubArea:    "magi",
	}

	p := memoryToProto(m)
	if p.Id != m.ID {
		t.Errorf("Id = %q, want %q", p.Id, m.ID)
	}
	if p.Content != m.Content {
		t.Errorf("Content = %q, want %q", p.Content, m.Content)
	}
	if p.Summary != m.Summary {
		t.Errorf("Summary = %q, want %q", p.Summary, m.Summary)
	}
	if p.Project != m.Project {
		t.Errorf("Project = %q, want %q", p.Project, m.Project)
	}
	if p.Type != m.Type {
		t.Errorf("Type = %q, want %q", p.Type, m.Type)
	}
	if p.Visibility != m.Visibility {
		t.Errorf("Visibility = %q, want %q", p.Visibility, m.Visibility)
	}
	if p.Source != m.Source {
		t.Errorf("Source = %q, want %q", p.Source, m.Source)
	}
	if p.SourceFile != m.SourceFile {
		t.Errorf("SourceFile = %q, want %q", p.SourceFile, m.SourceFile)
	}
	if p.ParentId != m.ParentID {
		t.Errorf("ParentId = %q, want %q", p.ParentId, m.ParentID)
	}
	if int(p.ChunkIndex) != m.ChunkIndex {
		t.Errorf("ChunkIndex = %d, want %d", p.ChunkIndex, m.ChunkIndex)
	}
	if p.CreatedAt != m.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", p.CreatedAt, m.CreatedAt)
	}
	if p.UpdatedAt != m.UpdatedAt {
		t.Errorf("UpdatedAt = %q, want %q", p.UpdatedAt, m.UpdatedAt)
	}
	if int(p.TokenCount) != m.TokenCount {
		t.Errorf("TokenCount = %d, want %d", p.TokenCount, m.TokenCount)
	}
	if len(p.Tags) != len(m.Tags) {
		t.Errorf("Tags len = %d, want %d", len(p.Tags), len(m.Tags))
	}
	if p.Speaker != m.Speaker {
		t.Errorf("Speaker = %q, want %q", p.Speaker, m.Speaker)
	}
	if p.Area != m.Area {
		t.Errorf("Area = %q, want %q", p.Area, m.Area)
	}
	if p.SubArea != m.SubArea {
		t.Errorf("SubArea = %q, want %q", p.SubArea, m.SubArea)
	}
}

func TestMemoriesToProto(t *testing.T) {
	memories := []*db.Memory{
		{ID: "a", Content: "first"},
		{ID: "b", Content: "second"},
		{ID: "c", Content: "third"},
	}
	protos := memoriesToProto(memories)
	if len(protos) != 3 {
		t.Fatalf("len = %d, want 3", len(protos))
	}
	for i, p := range protos {
		if p.Id != memories[i].ID {
			t.Errorf("[%d] Id = %q, want %q", i, p.Id, memories[i].ID)
		}
	}
}

func TestMemoriesToProto_Empty(t *testing.T) {
	protos := memoriesToProto(nil)
	if len(protos) != 0 {
		t.Errorf("len = %d, want 0", len(protos))
	}
}

func TestHybridResultsToProto(t *testing.T) {
	results := []*db.HybridResult{
		{
			Memory:        &db.Memory{ID: "r1", Content: "result one"},
			RRFScore:      0.95,
			VecRank:       1,
			BM25Rank:      2,
			Distance:      0.1,
			Score:         0.9,
			RecencyWeight: 0.8,
			WeightedScore: 0.72,
		},
		{
			Memory:   &db.Memory{ID: "r2", Content: "result two"},
			RRFScore: 0.5,
			VecRank:  3,
			BM25Rank: 1,
			Distance: 0.3,
			Score:    0.7,
		},
	}
	protos := hybridResultsToProto(results)
	if len(protos) != 2 {
		t.Fatalf("len = %d, want 2", len(protos))
	}
	p := protos[0]
	if p.Memory.Id != "r1" {
		t.Errorf("Memory.Id = %q, want r1", p.Memory.Id)
	}
	if p.RrfScore != 0.95 {
		t.Errorf("RrfScore = %f, want 0.95", p.RrfScore)
	}
	if p.VecRank != 1 {
		t.Errorf("VecRank = %d, want 1", p.VecRank)
	}
	if p.Bm25Rank != 2 {
		t.Errorf("Bm25Rank = %d, want 2", p.Bm25Rank)
	}
	if p.Distance != 0.1 {
		t.Errorf("Distance = %f, want 0.1", p.Distance)
	}
	if p.Score != 0.9 {
		t.Errorf("Score = %f, want 0.9", p.Score)
	}
	if p.RecencyWeight != 0.8 {
		t.Errorf("RecencyWeight = %f, want 0.8", p.RecencyWeight)
	}
	if p.WeightedScore != 0.72 {
		t.Errorf("WeightedScore = %f, want 0.72", p.WeightedScore)
	}
}

func TestHybridResultsToProto_Empty(t *testing.T) {
	protos := hybridResultsToProto(nil)
	if len(protos) != 0 {
		t.Errorf("len = %d, want 0", len(protos))
	}
}

// ---------------------------------------------------------------------------
// formatConversationContent
// ---------------------------------------------------------------------------

func TestFormatConversationContent_Full(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel:     "slack-general",
		Summary:     "We decided to use Go",
		SessionKey:  "sess-abc",
		StartedAt:   "2025-01-01T10:00:00Z",
		EndedAt:     "2025-01-01T11:00:00Z",
		TurnCount:   10,
		Topics:      []string{"golang", "architecture"},
		Decisions:   []string{"Use Go for backend", "Use gRPC for transport"},
		ActionItems: []string{"Write proto files", "Set up CI"},
	}

	content := formatConversationContent(req)

	assertContains(t, content, "Conversation on slack-general")
	assertContains(t, content, "(session: sess-abc)")
	assertContains(t, content, "Time: 2025-01-01T10:00:00Z to 2025-01-01T11:00:00Z")
	assertContains(t, content, "Turns: 10")
	assertContains(t, content, "Topics: golang, architecture")
	assertContains(t, content, "We decided to use Go")
	assertContains(t, content, "Decisions:")
	assertContains(t, content, "- Use Go for backend")
	assertContains(t, content, "- Use gRPC for transport")
	assertContains(t, content, "Action Items:")
	assertContains(t, content, "- Write proto files")
	assertContains(t, content, "- Set up CI")
}

func TestFormatConversationContent_Minimal(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel: "discord",
		Summary: "Quick sync",
	}

	content := formatConversationContent(req)

	assertContains(t, content, "Conversation on discord")
	assertContains(t, content, "Quick sync")

	// Should NOT contain optional sections
	if strings.Contains(content, "session:") {
		t.Error("should not contain session when empty")
	}
	if strings.Contains(content, "Turns:") {
		t.Error("should not contain turns when zero")
	}
	if strings.Contains(content, "Topics:") {
		t.Error("should not contain topics when empty")
	}
	if strings.Contains(content, "Decisions:") {
		t.Error("should not contain decisions when empty")
	}
	if strings.Contains(content, "Action Items:") {
		t.Error("should not contain action items when empty")
	}
}

func TestFormatConversationContent_PartialTime(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel:   "test",
		Summary:   "partial",
		StartedAt: "2025-01-01T10:00:00Z",
		// EndedAt intentionally empty
	}

	content := formatConversationContent(req)
	assertContains(t, content, "Time: 2025-01-01T10:00:00Z to ")
}

// ---------------------------------------------------------------------------
// Auth interceptor edge cases (tested directly, not via transport)
// ---------------------------------------------------------------------------

func TestAuthInterceptor_MissingMetadata(t *testing.T) {
	interceptor := AuthInterceptor("secret")
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	// context with no metadata at all
	_, err := interceptor(context.Background(), nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Remember"}, handler)
	assertGRPCCode(t, err, codes.Unauthenticated)
}

func TestAuthInterceptor_InvalidFormat(t *testing.T) {
	interceptor := AuthInterceptor("secret")
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	// "Basic" instead of "Bearer"
	md := map[string]string{"authorization": "Basic secret"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(md))
	_, err := interceptor(ctx, nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Remember"}, handler)
	assertGRPCCode(t, err, codes.Unauthenticated)
}

func TestAuthInterceptor_EmptyToken_DevMode(t *testing.T) {
	interceptor := AuthInterceptor("") // dev mode
	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	_, err := interceptor(context.Background(), nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Remember"}, handler)
	if err != nil {
		t.Fatalf("expected no error in dev mode: %v", err)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestAuthInterceptor_HealthBypass(t *testing.T) {
	interceptor := AuthInterceptor("secret")
	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	_, err := interceptor(context.Background(), nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Health"}, handler)
	if err != nil {
		t.Fatalf("health should bypass auth: %v", err)
	}
	if !called {
		t.Error("handler should have been called for health")
	}
}

func TestAuthInterceptor_ValidToken(t *testing.T) {
	interceptor := AuthInterceptor("my-secret")
	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	md := map[string]string{"authorization": "Bearer my-secret"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(md))
	_, err := interceptor(ctx, nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Remember"}, handler)
	if err != nil {
		t.Fatalf("expected no error with valid token: %v", err)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestAuthInterceptor_WrongToken(t *testing.T) {
	interceptor := AuthInterceptor("correct-token")
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	md := map[string]string{"authorization": "Bearer wrong-token"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(md))
	_, err := interceptor(ctx, nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Remember"}, handler)
	assertGRPCCode(t, err, codes.Unauthenticated)
}

func TestAuthInterceptor_MissingAuthHeader(t *testing.T) {
	interceptor := AuthInterceptor("secret")
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	// metadata present but no authorization key
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{"other": "value"}))
	_, err := interceptor(ctx, nil, &ggrpc.UnaryServerInfo{FullMethod: "/memory.v1.MemoryService/Remember"}, handler)
	assertGRPCCode(t, err, codes.Unauthenticated)
}

// ---------------------------------------------------------------------------
// Integration: Remember -> List -> Forget -> List
// ---------------------------------------------------------------------------

func TestRememberListForgetRoundTrip(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	// Remember
	rem, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "round trip test content",
		Project: "roundtrip",
		Tags:    []string{"roundtrip-tag"},
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// List and verify it exists
	list, err := srv.List(ctx, &pb.ListRequest{Project: "roundtrip"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Memories) != 1 {
		t.Fatalf("got %d memories, want 1", len(list.Memories))
	}
	if list.Memories[0].Id != rem.Id {
		t.Errorf("listed id = %q, want %q", list.Memories[0].Id, rem.Id)
	}
	if list.Memories[0].Content != "round trip test content" {
		t.Errorf("content = %q", list.Memories[0].Content)
	}

	// Forget
	_, err = srv.Forget(ctx, &pb.ForgetRequest{Id: rem.Id})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// List again — should be empty (archived)
	list2, err := srv.List(ctx, &pb.ListRequest{Project: "roundtrip"})
	if err != nil {
		t.Fatalf("List after forget: %v", err)
	}
	if len(list2.Memories) != 0 {
		t.Errorf("got %d memories after forget, want 0", len(list2.Memories))
	}
}

// ---------------------------------------------------------------------------
// Integration: CreateConversation -> SearchConversations
// ---------------------------------------------------------------------------

func TestCreateAndSearchConversations(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	conv, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel:    "slack",
		Summary:    "Memory search architecture discussion",
		Topics:     []string{"search", "vector"},
		Decisions:  []string{"Use cosine similarity"},
		TurnCount:  5,
		SessionKey: "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if !conv.Ok || conv.Id == "" {
		t.Fatalf("expected ok with id, got ok=%v id=%q", conv.Ok, conv.Id)
	}

	// Search should find it
	search, err := srv.SearchConversations(ctx, &pb.SearchConversationsRequest{
		Query:   "architecture",
		Channel: "slack",
	})
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if search == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// NewServer
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := NewServer(nil, nil, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertGRPCCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != want {
		t.Errorf("code = %s, want %s (msg: %s)", st.Code(), want, st.Message())
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
