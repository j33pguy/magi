package vcs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

type rebuildEmbedder struct{}

func (r *rebuildEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	if text != "" {
		emb[0] = float32(len(text))
	}
	return emb, nil
}

func (r *rebuildEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		emb, _ := r.Embed(context.Background(), text)
		out[i] = emb
	}
	return out, nil
}

func (r *rebuildEmbedder) Dimensions() int { return 384 }

func TestRebuildDBRestoresMemoriesContextsAndLinks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	repo, err := Init(tempConfig(t))
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	memoryData, err := MemoryToJSON(&db.Memory{
		ID:         "mem1",
		Content:    "rebuild me",
		Project:    "github.com/j33pguy/magi",
		Type:       "memory",
		Visibility: "internal",
		Source:     "api",
		CreatedAt:  "2026-04-15 23:00:00",
		UpdatedAt:  "2026-04-15 23:00:00",
		TokenCount: 2,
		Tags:       []string{"repo:j33pguy/magi", "alpha"},
	})
	if err != nil {
		t.Fatalf("MemoryToJSON: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo.path, "memories", "mem1.json"), memoryData, 0o644); err != nil {
		t.Fatalf("WriteFile memory: %v", err)
	}

	contextData, err := ContextToJSON(&db.MemoryContextRecord{
		MemoryID:                "mem1",
		Repository:              db.RepositoryRecord{CanonicalName: "j33pguy/magi", Owner: "j33pguy", Name: "magi"},
		ScopeMachine:            "gilfoyle",
		ProvenanceTransport:     "http",
		ProvenanceHumanAuthored: true,
	})
	if err != nil {
		t.Fatalf("ContextToJSON: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo.path, "contexts", "mem1.json"), contextData, 0o644); err != nil {
		t.Fatalf("WriteFile context: %v", err)
	}

	linkData, err := LinksToJSON([]*db.MemoryLink{{ID: "lnk1", FromID: "mem1", ToID: "mem1", Relation: "related_to", Weight: 1, CreatedAt: "2026-04-15 23:01:00"}})
	if err != nil {
		t.Fatalf("LinksToJSON: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo.path, "links", "mem1.json"), linkData, 0o644); err != nil {
		t.Fatalf("WriteFile link: %v", err)
	}

	client, err := db.NewSQLiteClient(filepath.Join(t.TempDir(), "rebuild.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	defer client.Close()
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if err := RebuildDB(client.TursoClient, repo, &rebuildEmbedder{}, logger); err != nil {
		t.Fatalf("RebuildDB: %v", err)
	}

	mem, err := client.TursoClient.GetMemory("mem1")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem.Content != "rebuild me" {
		t.Fatalf("content = %q", mem.Content)
	}
	links, err := client.TursoClient.GetLinks(context.Background(), "mem1", "from")
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	if len(links) != 1 || links[0].ID != "lnk1" {
		t.Fatalf("unexpected rebuilt links: %+v", links)
	}

	var repoCanonical, machine, transport string
	var humanAuthored int
	err = client.TursoClient.DB.QueryRow(`
		SELECT COALESCE(r.canonical_name, ''), mc.scope_machine, mc.provenance_transport, mc.provenance_human_authored
		FROM memory_contexts mc
		LEFT JOIN repositories r ON r.id = mc.repository_id
		WHERE mc.memory_id = ?
	`, "mem1").Scan(&repoCanonical, &machine, &transport, &humanAuthored)
	if err != nil {
		t.Fatalf("QueryRow memory_contexts: %v", err)
	}
	if repoCanonical != "j33pguy/magi" || machine != "gilfoyle" || transport != "http" || humanAuthored != 1 {
		t.Fatalf("unexpected rebuilt context: repo=%q machine=%q transport=%q human=%d", repoCanonical, machine, transport, humanAuthored)
	}
}
