package remember

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/secretstore"
)

type testEmbedder struct{}

func (t *testEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	if len(text) > 0 {
		emb[0] = float32(len(text)) / 100.0
	}
	return emb, nil
}

func (t *testEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		emb, _ := t.Embed(context.Background(), text)
		out[i] = emb
	}
	return out, nil
}

func (t *testEmbedder) Dimensions() int { return 384 }

type stubSecretManager struct{}

func (s *stubSecretManager) BackendName() string { return "vault" }

func (s *stubSecretManager) Externalize(_ context.Context, _ string, content string) (*secretstore.ExternalizeResult, error) {
	return &secretstore.ExternalizeResult{
		RedactedContent: strings.ReplaceAll(content, "abc123", "[stored:vault://magi/test#api_key]"),
		Refs: []secretstore.Reference{
			{Backend: "vault", Path: "magi/test", Key: "api_key"},
		},
	}, nil
}

func (s *stubSecretManager) Resolve(_ context.Context, path, key string) (string, error) {
	return path + "#" + key, nil
}

func newRememberStore(t *testing.T) db.Store {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	client, err := db.NewSQLiteClient(filepath.Join(tmp, "remember.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return client.TursoClient
}

func TestRememberExternalizesSecretsWithManager(t *testing.T) {
	store := newRememberStore(t)
	result, err := Remember(context.Background(), store, &testEmbedder{}, Input{
		Content: "api_key=abc123",
		Project: "secret-proj",
	}, Options{
		Logger:        slog.Default(),
		TagMode:       TagModeFail,
		SecretManager: &stubSecretManager{},
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if result.Saved == nil {
		t.Fatal("expected saved memory")
	}
	if strings.Contains(result.Saved.Content, "abc123") {
		t.Fatalf("expected secret to be redacted, got %q", result.Saved.Content)
	}
	if !strings.Contains(result.Saved.Content, "[stored:vault://magi/test#api_key]") {
		t.Fatalf("expected stored ref in content, got %q", result.Saved.Content)
	}

	tags, err := store.GetTags(result.Saved.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	tagSet := map[string]bool{}
	for _, tag := range tags {
		tagSet[tag] = true
	}
	if !tagSet["secret_backend:vault"] {
		t.Fatalf("expected secret backend tag, got %v", tags)
	}
	if !tagSet["secret_ref:magi/test#api_key"] {
		t.Fatalf("expected secret ref tag, got %v", tags)
	}
}

func TestRememberRejectsSecretsWithoutManager(t *testing.T) {
	store := newRememberStore(t)
	_, err := Remember(context.Background(), store, &testEmbedder{}, Input{
		Content: "api_key=abc123",
		Project: "secret-proj",
	}, Options{
		Logger:  slog.Default(),
		TagMode: TagModeFail,
	})
	if err == nil {
		t.Fatal("expected secret error")
	}
	if _, ok := err.(*SecretError); !ok {
		t.Fatalf("expected SecretError, got %T", err)
	}
}

func TestRememberDedupesWithinProject(t *testing.T) {
	store := newRememberStore(t)
	embedder := &testEmbedder{}
	opts := Options{Logger: slog.Default(), TagMode: TagModeWarn}

	first, err := Remember(context.Background(), store, embedder, Input{
		Content: "same content for same project",
		Project: "proj-a",
	}, opts)
	if err != nil {
		t.Fatalf("first remember: %v", err)
	}
	if first.Deduplicated {
		t.Fatal("first remember should not dedupe")
	}

	second, err := Remember(context.Background(), store, embedder, Input{
		Content: "same content for same project",
		Project: "proj-a",
	}, opts)
	if err != nil {
		t.Fatalf("second remember: %v", err)
	}
	if !second.Deduplicated {
		t.Fatal("second remember should dedupe within same project")
	}

	mems, err := store.ListMemories(&db.MemoryFilter{Project: "proj-a", Limit: 10})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("memories in proj-a = %d, want 1", len(mems))
	}
}

func TestRememberDoesNotDedupeAcrossProjects(t *testing.T) {
	store := newRememberStore(t)
	embedder := &testEmbedder{}
	opts := Options{Logger: slog.Default(), TagMode: TagModeWarn}

	_, err := Remember(context.Background(), store, embedder, Input{
		Content: "same content across projects",
		Project: "proj-a",
	}, opts)
	if err != nil {
		t.Fatalf("remember proj-a: %v", err)
	}

	second, err := Remember(context.Background(), store, embedder, Input{
		Content: "same content across projects",
		Project: "proj-b",
	}, opts)
	if err != nil {
		t.Fatalf("remember proj-b: %v", err)
	}
	if second.Deduplicated {
		t.Fatal("remember in proj-b should not dedupe against proj-a")
	}

	memA, err := store.ListMemories(&db.MemoryFilter{Project: "proj-a", Limit: 10})
	if err != nil {
		t.Fatalf("ListMemories proj-a: %v", err)
	}
	memB, err := store.ListMemories(&db.MemoryFilter{Project: "proj-b", Limit: 10})
	if err != nil {
		t.Fatalf("ListMemories proj-b: %v", err)
	}
	if len(memA) != 1 || len(memB) != 1 {
		t.Fatalf("expected 1 memory per project, got proj-a=%d proj-b=%d", len(memA), len(memB))
	}
}
