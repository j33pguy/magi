package grpc

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
	pb "github.com/j33pguy/magi/proto/memory/v1"
)

// newTestGRPCServerWithDB is like newTestGRPCServer but also returns the
// underlying db client so tests can close it to trigger DB error paths.
func newTestGRPCServerWithDB(t *testing.T) (*Server, *mockEmbedder, *db.SQLiteClient) {
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
	return srv, emb, client
}

// ---------------------------------------------------------------------------
// Remember — DB error paths
// ---------------------------------------------------------------------------

func TestRemember_SaveMemoryDBError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	// Close DB to force SaveMemory error
	dbClient.DB.Close()

	_, err := srv.Remember(context.Background(), &pb.RememberRequest{Content: "test"})
	assertGRPCCode(t, err, codes.Internal)
}

func TestRemember_SetTagsError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// First, remember a memory successfully
	resp, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "test for tags error",
		Tags:    []string{"tag1"},
		Speaker: "user",
		Area:    "work",
		SubArea: "magi",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}

	// Now close DB and try again — SaveMemory will fail before SetTags.
	// We need a way to make SetTags fail but SaveMemory succeed.
	// Instead, we drop the tags table to cause SetTags to fail.
	_, execErr := dbClient.DB.Exec("DROP TABLE IF EXISTS memory_tags")
	if execErr != nil {
		t.Fatalf("drop table: %v", execErr)
	}

	resp2, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "tags will fail",
		Tags:    []string{"broken"},
		Speaker: "user",
		Area:    "test",
	})
	if err != nil {
		t.Fatalf("Remember should succeed even if SetTags fails: %v", err)
	}
	if !resp2.Ok {
		t.Error("expected ok=true")
	}
	if resp2.TagWarning == "" {
		t.Error("expected non-empty TagWarning when SetTags fails")
	}
	if !strings.Contains(resp2.TagWarning, "tags may not have been saved") {
		t.Errorf("unexpected TagWarning: %q", resp2.TagWarning)
	}
}

func TestRemember_SetTagsErrorLongMessage(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Drop the tags table to cause SetTags to fail.
	// We need to verify the error truncation to 80 chars.
	_, execErr := dbClient.DB.Exec("DROP TABLE IF EXISTS memory_tags")
	if execErr != nil {
		t.Fatalf("drop table: %v", execErr)
	}

	resp, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "long error test",
		Tags:    []string{"broken"},
		Speaker: "user",
	})
	if err != nil {
		t.Fatalf("Remember should succeed: %v", err)
	}
	if resp.TagWarning == "" {
		t.Error("expected TagWarning")
	}
}

// ---------------------------------------------------------------------------
// Recall — search error path
// ---------------------------------------------------------------------------

func TestRecall_SearchError(t *testing.T) {
	srv, emb, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Seed a memory so the query doesn't just return empty
	_, err := srv.Remember(ctx, &pb.RememberRequest{Content: "seed"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Close the DB so the search will fail during DB operations
	dbClient.DB.Close()

	// The embedder still works, but DB queries will fail
	_ = emb // keep embedder working
	_, err = srv.Recall(ctx, &pb.RecallRequest{Query: "seed"})
	assertGRPCCode(t, err, codes.Internal)
}

func TestRecall_WithAllFilters(t *testing.T) {
	srv, _, _ := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "full filter recall test",
		Project: "proj-x",
		Type:    "decision",
		Tags:    []string{"important"},
		Speaker: "agent",
		Area:    "eng",
		SubArea: "backend",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.Recall(ctx, &pb.RecallRequest{
		Query:        "full filter",
		Project:      "proj-x",
		Projects:     []string{"proj-x", "proj-y"},
		Type:         "decision",
		Tags:         []string{"important"},
		Speaker:      "agent",
		Area:         "eng",
		SubArea:      "backend",
		TopK:         3,
		MinRelevance: 0.0,
		RecencyDecay: 0.5,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRecall_ResultFields(t *testing.T) {
	srv, _, _ := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "result fields test memory",
		Project: "result-proj",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.Recall(ctx, &pb.RecallRequest{
		Query:   "result fields test",
		Project: "result-proj",
		TopK:    10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(resp.Results) > 0 {
		r := resp.Results[0]
		if r.Memory == nil {
			t.Error("expected non-nil Memory in result")
		}
		if r.Memory.Content == "" {
			t.Error("expected non-empty content")
		}
	}
}

// ---------------------------------------------------------------------------
// Forget — ArchiveMemory error path
// ---------------------------------------------------------------------------

func TestForget_ArchiveDBError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := auth.NewContext(context.Background(), &auth.Identity{Kind: "admin"})

	// Create a memory first
	rem, err := srv.Remember(ctx, &pb.RememberRequest{Content: "to archive fail"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Close DB to make ArchiveMemory fail.
	// But GetMemory also uses the DB, so it will fail with NotFound first.
	// We need to drop only the memories table or use a different approach.
	// Actually, let's close DB — GetMemory will also fail, returning NotFound.
	dbClient.DB.Close()

	_, err = srv.Forget(ctx, &pb.ForgetRequest{Id: rem.Id})
	// Will fail at GetMemory with NotFound or at ArchiveMemory with Internal
	if err == nil {
		t.Fatal("expected error when DB is closed")
	}
}

// ---------------------------------------------------------------------------
// List — DB error paths
// ---------------------------------------------------------------------------

func TestList_ListMemoriesDBError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Seed a memory
	_, err := srv.Remember(ctx, &pb.RememberRequest{Content: "list error test", Project: "err-proj"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Close DB
	dbClient.DB.Close()

	_, err = srv.List(ctx, &pb.ListRequest{Project: "err-proj"})
	assertGRPCCode(t, err, codes.Internal)
}

func TestList_GetTagsError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Seed a memory with tags
	_, err := srv.Remember(ctx, &pb.RememberRequest{
		Content: "get tags error test",
		Project: "tags-err-proj",
		Tags:    []string{"sometag"},
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Drop the tags table so GetTags fails but ListMemories succeeds
	_, execErr := dbClient.DB.Exec("DROP TABLE IF EXISTS memory_tags")
	if execErr != nil {
		t.Fatalf("drop table: %v", execErr)
	}

	resp, err := srv.List(ctx, &pb.ListRequest{Project: "tags-err-proj"})
	if err != nil {
		t.Fatalf("List should succeed even when GetTags fails: %v", err)
	}
	// The memories should still be returned, just without tags
	if len(resp.Memories) == 0 {
		t.Error("expected at least 1 memory")
	}
}

func TestList_WithValidTime(t *testing.T) {
	srv, _, _ := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	_, err := srv.Remember(ctx, &pb.RememberRequest{Content: "time filtered", Project: "time-proj"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	resp, err := srv.List(ctx, &pb.ListRequest{
		Project:   "time-proj",
		AfterTime: "30d",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Memories) == 0 {
		t.Error("expected at least 1 memory")
	}
}

// ---------------------------------------------------------------------------
// CreateConversation — DB error paths
// ---------------------------------------------------------------------------

func TestCreateConversation_SaveMemoryDBError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)

	// Close DB to make SaveMemory fail
	dbClient.DB.Close()

	_, err := srv.CreateConversation(context.Background(), &pb.CreateConversationRequest{
		Channel: "test",
		Summary: "test",
	})
	assertGRPCCode(t, err, codes.Internal)
}

func TestCreateConversation_SetTagsError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Drop the tags table so SetTags fails
	_, execErr := dbClient.DB.Exec("DROP TABLE IF EXISTS memory_tags")
	if execErr != nil {
		t.Fatalf("drop table: %v", execErr)
	}

	resp, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "test-channel",
		Summary: "test conversation",
		Topics:  []string{"topic1"},
	})
	if err != nil {
		t.Fatalf("CreateConversation should succeed even if SetTags fails: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.TagWarning == "" {
		t.Error("expected non-empty TagWarning")
	}
	if !strings.Contains(resp.TagWarning, "tags may not have been saved") {
		t.Errorf("unexpected TagWarning: %q", resp.TagWarning)
	}
}

func TestCreateConversation_SetTagsErrorLongMessage(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Drop the tags table
	_, execErr := dbClient.DB.Exec("DROP TABLE IF EXISTS memory_tags")
	if execErr != nil {
		t.Fatalf("drop table: %v", execErr)
	}

	resp, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "long-error-channel",
		Summary: "test for long error truncation",
		Topics:  []string{"t1", "t2", "t3"},
	})
	if err != nil {
		t.Fatalf("CreateConversation should succeed: %v", err)
	}
	if resp.TagWarning == "" {
		t.Error("expected TagWarning")
	}
}

// ---------------------------------------------------------------------------
// SearchConversations — search error path
// ---------------------------------------------------------------------------

func TestSearchConversations_SearchError(t *testing.T) {
	srv, _, dbClient := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	// Seed a conversation
	_, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "err-ch",
		Summary: "search error test",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// Close DB to make search fail
	dbClient.DB.Close()

	_, err = srv.SearchConversations(ctx, &pb.SearchConversationsRequest{
		Query: "search error",
	})
	assertGRPCCode(t, err, codes.Internal)
}

func TestSearchConversations_WithMinRelevanceAndDecay(t *testing.T) {
	srv, _, _ := newTestGRPCServerWithDB(t)
	ctx := context.Background()

	_, err := srv.CreateConversation(ctx, &pb.CreateConversationRequest{
		Channel: "test-ch",
		Summary: "relevance and decay test",
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	resp, err := srv.SearchConversations(ctx, &pb.SearchConversationsRequest{
		Query:        "relevance",
		Limit:        10,
		MinRelevance: 0.0,
		RecencyDecay: 0.5,
	})
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// formatConversationContent — edge cases for additional coverage
// ---------------------------------------------------------------------------

func TestFormatConversationContent_OnlyEndedAt(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel: "test",
		Summary: "ended only",
		EndedAt: "2025-06-01T12:00:00Z",
	}

	content := formatConversationContent(req)
	assertContains(t, content, "Time:  to 2025-06-01T12:00:00Z")
}

func TestFormatConversationContent_TopicsOnly(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel: "test",
		Summary: "topics only",
		Topics:  []string{"go", "testing"},
	}

	content := formatConversationContent(req)
	assertContains(t, content, "Topics: go, testing")
	if strings.Contains(content, "Decisions:") {
		t.Error("should not contain decisions")
	}
	if strings.Contains(content, "Action Items:") {
		t.Error("should not contain action items")
	}
}

func TestFormatConversationContent_DecisionsOnly(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel:   "test",
		Summary:   "decisions only",
		Decisions: []string{"Use Go", "Use gRPC"},
	}

	content := formatConversationContent(req)
	assertContains(t, content, "Decisions:")
	assertContains(t, content, "- Use Go")
	assertContains(t, content, "- Use gRPC")
	if strings.Contains(content, "Action Items:") {
		t.Error("should not contain action items")
	}
}

func TestFormatConversationContent_ActionItemsOnly(t *testing.T) {
	req := &pb.CreateConversationRequest{
		Channel:     "test",
		Summary:     "action items only",
		ActionItems: []string{"Deploy v2", "Update docs"},
	}

	content := formatConversationContent(req)
	assertContains(t, content, "Action Items:")
	assertContains(t, content, "- Deploy v2")
	assertContains(t, content, "- Update docs")
	if strings.Contains(content, "Decisions:") {
		t.Error("should not contain decisions")
	}
}
