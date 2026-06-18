package remember

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/cache"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
	localnode "github.com/j33pguy/magi/internal/node/local"
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

func newRememberStore(t *testing.T) *db.Client {
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

func fetchRememberContextRow(t *testing.T, store *db.Client, memoryID string) (string, string, string, bool) {
	t.Helper()
	var canonicalName, scopeMachine, transport string
	var humanAuthored int
	err := store.DB.QueryRow(`
		SELECT COALESCE(r.canonical_name, ''), mc.scope_machine, mc.provenance_transport, mc.provenance_human_authored
		FROM memory_contexts mc
		LEFT JOIN repositories r ON r.id = mc.repository_id
		WHERE mc.memory_id = ?
	`, memoryID).Scan(&canonicalName, &scopeMachine, &transport, &humanAuthored)
	if err != nil {
		t.Fatalf("QueryRow memory_contexts: %v", err)
	}
	return canonicalName, scopeMachine, transport, humanAuthored == 1
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

func TestBuildTagsAddsRepoFacet(t *testing.T) {
	tags := BuildTags(Input{
		Project: "github.com/j33pguy/magi",
		Speaker: "assistant",
		Area:    "project",
		SubArea: "memory",
		Tags:    []string{"source:mcp"},
	})

	tagSet := map[string]bool{}
	for _, tag := range tags {
		tagSet[tag] = true
	}
	if !tagSet["repo:j33pguy/magi"] {
		t.Fatalf("expected repo facet, got %v", tags)
	}
	if !tagSet["speaker:assistant"] || !tagSet["area:project"] || !tagSet["sub_area:memory"] {
		t.Fatalf("expected derived taxonomy tags, got %v", tags)
	}
}

func assertRememberRepoContextPersisted(t *testing.T, queryStore db.Store, inspectStore *db.Client) {
	t.Helper()
	result, err := Remember(context.Background(), queryStore, &testEmbedder{}, Input{
		Content:       "remember repo-aware context",
		Project:       "github.com/j33pguy/magi",
		Machine:       "gilfoyle",
		Transport:     "http",
		HumanAuthored: true,
	}, Options{
		Logger:  slog.Default(),
		TagMode: TagModeFail,
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	tags, err := inspectStore.GetTags(result.Saved.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	found := false
	for _, tag := range tags {
		if tag == "repo:j33pguy/magi" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected repo facet in persisted tags, got %v", tags)
	}

	canonicalName, scopeMachine, transport, humanAuthored := fetchRememberContextRow(t, inspectStore, result.Saved.ID)
	if canonicalName != "j33pguy/magi" || scopeMachine != "gilfoyle" || transport != "http" || !humanAuthored {
		t.Fatalf("unexpected persisted context: canonical=%q machine=%q transport=%q human=%v", canonicalName, scopeMachine, transport, humanAuthored)
	}
}

func TestRememberPersistsRepoFacet(t *testing.T) {
	store := newRememberStore(t)
	assertRememberRepoContextPersisted(t, store, store)
}

func TestRememberPersistsRepoFacetThroughCoordinatedAndCacheWrappers(t *testing.T) {
	base := newRememberStore(t)
	cfg := node.DefaultConfig()
	coord := localnode.NewCoordinator(cfg, base, slog.Default())
	if err := coord.Start(context.Background()); err != nil {
		t.Fatalf("Start coordinator: %v", err)
	}
	defer coord.Stop()

	wrapped := cache.NewStore(localnode.NewCoordinatedStore(coord, base), cache.Config{
		Enabled:    true,
		QueryTTL:   time.Minute,
		MemorySize: 16,
	})
	defer wrapped.Close()

	assertRememberRepoContextPersisted(t, wrapped, base)
}

func TestBuildTagsNormalizesSSHRepoFacet(t *testing.T) {
	tags := BuildTags(Input{
		Project: "git@github.com:j33pguy/magi.git",
	})

	found := false
	for _, tag := range tags {
		if tag == "repo:j33pguy/magi" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected normalized SSH repo facet, got %v", tags)
	}
}

func TestBuildTagsInfersRepoFacetFromSourceFileGitOrigin(t *testing.T) {
	repoDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("git", "init")
	run("git", "remote", "add", "origin", "git@github.com:j33pguy/magi.git")

	sourceFile := filepath.Join(repoDir, "internal", "remember", "metadata.go")
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(sourceFile, []byte("package remember\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tags := BuildTags(Input{SourceFile: sourceFile})
	found := false
	for _, tag := range tags {
		if tag == "repo:j33pguy/magi" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected repo facet inferred from git source file, got %v", tags)
	}
}

func TestRememberSkipsCrossProjectDedup(t *testing.T) {
	store := newRememberStore(t)
	first, err := Remember(context.Background(), store, &testEmbedder{}, Input{
		Content: "same content, different project",
		Project: "proj-a",
	}, Options{Logger: slog.Default(), TagMode: TagModeFail})
	if err != nil {
		t.Fatalf("first Remember: %v", err)
	}
	second, err := Remember(context.Background(), store, &testEmbedder{}, Input{
		Content: "same content, different project",
		Project: "proj-b",
	}, Options{Logger: slog.Default(), TagMode: TagModeFail})
	if err != nil {
		t.Fatalf("second Remember: %v", err)
	}
	if second.Deduplicated {
		t.Fatal("expected cross-project duplicate to be inserted, not deduplicated")
	}
	if second.Saved == nil || first.Saved == nil {
		t.Fatal("expected both memories to be saved")
	}
	if second.Saved.ID == first.Saved.ID {
		t.Fatal("expected distinct memory ids across projects")
	}
}
