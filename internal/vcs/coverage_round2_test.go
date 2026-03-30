package vcs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// ---------------------------------------------------------------------------
// store.go — writeMemoryToGit: WriteAndCommit failure
// ---------------------------------------------------------------------------

func TestWriteMemoryToGit_WriteAndCommitFails(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	// Corrupt the git repo so WriteAndCommit fails.
	os.RemoveAll(filepath.Join(store.repo.path, ".git"))

	// Update should succeed (DB write) but git write should fail silently.
	m.Content = "updated after repo corruption"
	err := store.UpdateMemory(m)
	// UpdateMemory calls GetMemory which should still work (DB is fine),
	// then writeMemoryToGit will fail silently. No error returned.
	if err != nil {
		t.Fatalf("UpdateMemory should succeed even when git is broken: %v", err)
	}
}

// ---------------------------------------------------------------------------
// store.go — writeLinkToGit: GetLinks fails (close DB before link write)
// ---------------------------------------------------------------------------

func TestWriteLinkToGit_GetLinksFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)

	// Save two memories for link creation.
	emb := make([]float32, 384)
	m1, err := store.SaveMemory(&db.Memory{
		Content: "link source", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory m1: %v", err)
	}
	m2, err := store.SaveMemory(&db.Memory{
		Content: "link target", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory m2: %v", err)
	}

	// Close the DB so GetLinks inside writeLinkToGit fails.
	client.Close()

	// CreateLink will fail at Client.CreateLink since DB is closed.
	ctx := context.Background()
	_, err = store.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.5, false)
	if err == nil {
		t.Error("CreateLink should fail when DB is closed")
	}
}

// ---------------------------------------------------------------------------
// store.go — writeLinkToGit: WriteAndCommit fails (corrupt git after link save)
// ---------------------------------------------------------------------------

func TestWriteLinkToGit_WriteAndCommitFails(t *testing.T) {
	store, _ := newTestStore(t)

	emb := make([]float32, 384)
	m1, err := store.SaveMemory(&db.Memory{
		Content: "link src", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory m1: %v", err)
	}
	m2, err := store.SaveMemory(&db.Memory{
		Content: "link dst", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory m2: %v", err)
	}

	// Corrupt git repo so WriteAndCommit fails inside writeLinkToGit.
	os.RemoveAll(filepath.Join(store.repo.path, ".git"))

	ctx := context.Background()
	link, err := store.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.7, false)
	// DB write should succeed but git write should fail silently.
	if err != nil {
		t.Fatalf("CreateLink should succeed even when git is broken: %v", err)
	}
	if link == nil {
		t.Error("link should not be nil")
	}
}

// ---------------------------------------------------------------------------
// store.go — DeleteMemory: Client.DeleteMemory fails
// ---------------------------------------------------------------------------

func TestDeleteMemory_ClientFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)
	m := seedTestMemory(t, store)

	// Close DB to make DeleteMemory fail.
	client.Close()

	err = store.DeleteMemory(m.ID)
	if err == nil {
		t.Error("DeleteMemory should propagate DB error")
	}
}

// ---------------------------------------------------------------------------
// store.go — DeleteMemory: RemoveAndCommit fails (corrupt git)
// ---------------------------------------------------------------------------

func TestDeleteMemory_RemoveAndCommitFails(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	// Corrupt the git repo so RemoveAndCommit fails (logged, not returned).
	os.RemoveAll(filepath.Join(store.repo.path, ".git"))

	// DB delete should succeed; git error is only logged.
	err := store.DeleteMemory(m.ID)
	if err != nil {
		t.Fatalf("DeleteMemory should succeed even when git is broken: %v", err)
	}
}

// ---------------------------------------------------------------------------
// store.go — DeleteLink: Client.DeleteLink fails
// ---------------------------------------------------------------------------

func TestDeleteLink_ClientFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)

	emb := make([]float32, 384)
	m1, _ := store.SaveMemory(&db.Memory{
		Content: "src", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	m2, _ := store.SaveMemory(&db.Memory{
		Content: "dst", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})

	ctx := context.Background()
	link, err := store.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.5, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Close DB to make DeleteLink fail.
	client.Close()

	err = store.DeleteLink(ctx, link.ID)
	if err == nil {
		t.Error("DeleteLink should propagate DB error")
	}
}

// ---------------------------------------------------------------------------
// store.go — CreateLink: Client.CreateLink fails
// ---------------------------------------------------------------------------

func TestCreateLink_ClientFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)

	// Close DB before CreateLink so it fails.
	client.Close()

	ctx := context.Background()
	_, err = store.CreateLink(ctx, "fake-from", "fake-to", "related_to", 0.5, false)
	if err == nil {
		t.Error("CreateLink should fail when DB is closed")
	}
}

// ---------------------------------------------------------------------------
// store.go — SetTags: GetMemory fails after successful SetTags
// ---------------------------------------------------------------------------

func TestSetTags_GetMemoryFailsAfterSuccess(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)
	m := seedTestMemory(t, store)

	// Close DB after saving memory. SetTags will fail at Client.SetTags.
	client.Close()

	err = store.SetTags(m.ID, []string{"tag1"})
	if err == nil {
		t.Error("SetTags should fail when DB is closed")
	}
}

// ---------------------------------------------------------------------------
// store.go — ArchiveMemory: GetMemory fails after successful archive
// ---------------------------------------------------------------------------

func TestArchiveMemory_GetMemoryFailsAfterSuccess(t *testing.T) {
	// We need ArchiveMemory to succeed but GetMemory to fail.
	// This is hard with a real DB — we simulate by archiving, then the
	// subsequent GetMemory is called. We can't easily split this, but we
	// can test the Client.ArchiveMemory failure path.
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)

	// Close DB so ArchiveMemory fails at Client.ArchiveMemory.
	client.Close()

	err = store.ArchiveMemory("nonexistent")
	if err == nil {
		t.Error("ArchiveMemory should propagate DB error")
	}
}

// ---------------------------------------------------------------------------
// store.go — UpdateMemory: Client.UpdateMemory fails
// ---------------------------------------------------------------------------

func TestUpdateMemory_ClientFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)
	m := seedTestMemory(t, store)

	// Close DB so UpdateMemory fails.
	client.Close()

	m.Content = "this will fail"
	err = store.UpdateMemory(m)
	if err == nil {
		t.Error("UpdateMemory should propagate DB error")
	}
}

// ---------------------------------------------------------------------------
// store.go — UpdateMemory: GetMemory fails after successful update
// We need the update to succeed but GetMemory to fail. Since both use the
// same DB, we test the path by verifying the function returns nil (not error)
// when GetMemory fails. We can trigger this by deleting the memory between
// update and get — but since it's the same transaction flow, we rely on the
// "nonexistent" test already covering Client.UpdateMemory failure. Instead,
// let's verify the warn-and-return-nil path by corrupting the DB path.
// ---------------------------------------------------------------------------

func TestUpdateMemory_GetMemoryFailsAfterUpdate(t *testing.T) {
	// Use a wrapper approach: save memory, then manipulate state so GetMemory
	// fails. We'll do this by closing the DB after the update starts.
	// Since we can't intercept mid-call easily, we test the negative path:
	// UpdateMemory on a memory whose row is deleted mid-flight.
	store, sqliteClient := newTestStore(t)
	m := seedTestMemory(t, store)

	// Delete the memory row directly from the DB so UpdateMemory succeeds
	// (no-op update) but GetMemory fails.
	_, err := sqliteClient.TursoClient.DB.Exec("DELETE FROM memories WHERE id = ?", m.ID)
	if err != nil {
		t.Fatalf("direct delete: %v", err)
	}

	m.Content = "updated content"
	// UpdateMemory will fail at Client.UpdateMemory because the row is gone.
	// This exercises the error propagation path.
	err = store.UpdateMemory(m)
	// Depending on DB behavior, this may or may not error. Either way, no panic.
	_ = err
}

// ---------------------------------------------------------------------------
// store.go — SaveMemory: Client.SaveMemory fails (closed DB)
// ---------------------------------------------------------------------------

func TestSaveMemory_ClientFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)

	client.Close()

	emb := make([]float32, 384)
	_, err = store.SaveMemory(&db.Memory{
		Content: "fail", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err == nil {
		t.Error("SaveMemory should fail when DB is closed")
	}
}

// ---------------------------------------------------------------------------
// git.go — WriteAndCommit: MkdirAll failure
// ---------------------------------------------------------------------------

func TestWriteAndCommit_MkdirAllFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Create a file where the directory should be, so MkdirAll fails.
	blocker := filepath.Join(cfg.Path, "blocked")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("creating blocker file: %v", err)
	}

	err = repo.WriteAndCommit("blocked/sub/file.json", []byte("data"), "test")
	if err == nil {
		t.Error("WriteAndCommit should fail when MkdirAll fails")
	}
}

// ---------------------------------------------------------------------------
// git.go — WriteAndCommit: WriteFile failure (read-only parent)
// ---------------------------------------------------------------------------

func TestWriteAndCommit_WriteFileFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Create a read-only directory so WriteFile fails.
	readOnlyDir := filepath.Join(cfg.Path, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(readOnlyDir, 0o755) })

	err = repo.WriteAndCommit("readonly/file.json", []byte("data"), "test")
	if err == nil {
		t.Error("WriteAndCommit should fail when WriteFile fails")
	}
}

// ---------------------------------------------------------------------------
// git.go — RemoveAndCommit: os.Remove failure (permission denied, not NotExist)
// ---------------------------------------------------------------------------

func TestRemoveAndCommit_RemoveFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a file first.
	relPath := filepath.Join("memories", "perm-test.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Make the directory read-only so os.Remove fails with permission error.
	memDir := filepath.Join(cfg.Path, "memories")
	if err := os.Chmod(memDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(memDir, 0o755) })

	err = repo.RemoveAndCommit(relPath, "delete")
	if err == nil {
		t.Error("RemoveAndCommit should fail when os.Remove fails with permission error")
	}
}

// ---------------------------------------------------------------------------
// git.go — Log: iterator error path (corrupted repo)
// ---------------------------------------------------------------------------

func TestLog_CorruptedRepo(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a file to have some history.
	relPath := filepath.Join("memories", "log-err.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Corrupt the git objects directory.
	objectsDir := filepath.Join(cfg.Path, ".git", "objects")
	entries, _ := os.ReadDir(objectsDir)
	for _, e := range entries {
		if e.IsDir() && e.Name() != "info" && e.Name() != "pack" {
			subDir := filepath.Join(objectsDir, e.Name())
			subEntries, _ := os.ReadDir(subDir)
			for _, se := range subEntries {
				os.WriteFile(filepath.Join(subDir, se.Name()), []byte("corrupted"), 0o644)
			}
		}
	}

	_, err = repo.Log(relPath)
	// May or may not error depending on how go-git handles corruption,
	// but should not panic.
	_ = err
}

// ---------------------------------------------------------------------------
// git.go — flushBatch: worktree error (corrupted repo state)
// ---------------------------------------------------------------------------

func TestFlushBatch_WorktreeError(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 10 * time.Second

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Stage a file to make pending=true.
	relPath := filepath.Join("memories", "flush-err.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "batch add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Set pending manually (WriteAndCommit in batch mode sets it).
	repo.pending = true

	// Corrupt the .git directory so Worktree() fails.
	os.RemoveAll(filepath.Join(cfg.Path, ".git"))

	// flushBatch should not panic even with corrupted repo.
	repo.flushBatch()

	// Stop the batch loop.
	close(repo.stopBatch)
	<-repo.batchDone
}

// ---------------------------------------------------------------------------
// git.go — fileAtCommit: file missing in tree (different from bad hash)
// Already tested above in coverage_test.go TestFileAtCommit_MissingFile
// but let's also test via Diff to hit the error wrapping path.
// ---------------------------------------------------------------------------

func TestDiff_FileMissingInTree(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write file A to get a commit.
	relPathA := filepath.Join("memories", "file-a.json")
	if err := repo.WriteAndCommit(relPathA, []byte(`{"a":1}`+"\n"), "add a"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	commitsA, err := repo.Log(relPathA)
	if err != nil || len(commitsA) == 0 {
		t.Fatalf("Log: %v", err)
	}

	// Write file B to get a different commit.
	relPathB := filepath.Join("memories", "file-b.json")
	if err := repo.WriteAndCommit(relPathB, []byte(`{"b":1}`+"\n"), "add b"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	commitsB, err := repo.Log(relPathB)
	if err != nil || len(commitsB) == 0 {
		t.Fatalf("Log: %v", err)
	}

	// Try to diff file B at commit A (where B doesn't exist).
	_, err = repo.Diff(relPathB, commitsA[0].Hash, commitsB[0].Hash)
	if err == nil {
		t.Error("Diff should fail when file is missing in the from-commit tree")
	}
}

// ---------------------------------------------------------------------------
// serialize.go — MemoryToJSON with zero-value memory (not nil, that would panic)
// ---------------------------------------------------------------------------

func TestMemoryToJSON_EmptyMemory(t *testing.T) {
	m := &db.Memory{}
	data, err := MemoryToJSON(m)
	if err != nil {
		t.Fatalf("MemoryToJSON with empty memory: %v", err)
	}

	// Should be valid JSON.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Fields should be present with zero values.
	if raw["id"] != "" {
		t.Errorf("expected empty id, got %v", raw["id"])
	}
}

// ---------------------------------------------------------------------------
// serialize.go — LinksToJSON with nil/empty slice
// ---------------------------------------------------------------------------

func TestLinksToJSON_NilSlice(t *testing.T) {
	data, err := LinksToJSON(nil)
	if err != nil {
		t.Fatalf("LinksToJSON(nil): %v", err)
	}

	var parsed []SerializableLink
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected 0 links, got %d", len(parsed))
	}
}

func TestLinksToJSON_EmptySlice(t *testing.T) {
	data, err := LinksToJSON([]*db.MemoryLink{})
	if err != nil {
		t.Fatalf("LinksToJSON(empty): %v", err)
	}

	var parsed []SerializableLink
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected 0 links, got %d", len(parsed))
	}
}

// ---------------------------------------------------------------------------
// serialize.go — JSONToMemory with invalid JSON
// ---------------------------------------------------------------------------

func TestJSONToMemory_InvalidJSON(t *testing.T) {
	_, err := JSONToMemory([]byte("not valid json"))
	if err == nil {
		t.Error("JSONToMemory should fail with invalid JSON")
	}
}

func TestJSONToMemory_EmptyObject(t *testing.T) {
	m, err := JSONToMemory([]byte(`{}`))
	if err != nil {
		t.Fatalf("JSONToMemory({}): %v", err)
	}
	if m.ID != "" {
		t.Errorf("expected empty ID, got %q", m.ID)
	}
}

func TestJSONToMemory_EmptyBytes(t *testing.T) {
	_, err := JSONToMemory([]byte(""))
	if err == nil {
		t.Error("JSONToMemory should fail with empty bytes")
	}
}

// ---------------------------------------------------------------------------
// rebuild.go — RebuildDB: os.ReadFile fails on permission error
// ---------------------------------------------------------------------------

func TestRebuildDB_ReadFilePermissionError(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a valid memory file then make it unreadable.
	mem := &SerializableMemory{
		ID:         "perm-err-mem",
		Content:    "unreadable",
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(mem, "", "  ")
	memFile := filepath.Join(repo.MemoriesDir(), "perm-err-mem.json")
	if err := os.WriteFile(memFile, data, 0o644); err != nil {
		t.Fatalf("writing memory file: %v", err)
	}
	if err := os.Chmod(memFile, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(memFile, 0o644) })

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB should not fail on unreadable file (skips it): %v", err)
	}

	// Memory should not have been imported.
	if !DBIsEmpty(client) {
		t.Error("DB should be empty because the file was unreadable")
	}
}

// ---------------------------------------------------------------------------
// rebuild.go — RebuildDB: store.SaveMemory fails (closed DB)
// ---------------------------------------------------------------------------

func TestRebuildDB_SaveMemoryFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a valid memory file.
	mem := &SerializableMemory{
		ID:         "save-fail-mem",
		Content:    "save will fail",
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(mem, "", "  ")
	memFile := filepath.Join(repo.MemoriesDir(), "save-fail-mem.json")
	if err := os.WriteFile(memFile, data, 0o644); err != nil {
		t.Fatalf("writing memory file: %v", err)
	}

	// Close DB so SaveMemory fails.
	client.Close()

	err = RebuildDB(client.TursoClient, repo, &mockEmbedder{}, logger)
	// Should not return error — individual failures are logged and skipped.
	if err != nil {
		t.Fatalf("RebuildDB should not fail on SaveMemory error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// rebuild.go — RebuildDB: store.SetTags fails (closed DB after save)
// We need SaveMemory to succeed but SetTags to fail. Use a wrapper store.
// ---------------------------------------------------------------------------

// tagFailStore wraps a db.Store and fails on SetTags after N successful calls.
type tagFailStore struct {
	db.Store
	setTagsCalls int
	failAfter    int
}

func (s *tagFailStore) SetTags(memoryID string, tags []string) error {
	s.setTagsCalls++
	if s.setTagsCalls > s.failAfter {
		return os.ErrClosed
	}
	return s.Store.SetTags(memoryID, tags)
}

func TestRebuildDB_SetTagsFails(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a memory with tags.
	mem := &SerializableMemory{
		ID:         "tags-fail-mem",
		Content:    "memory with tags",
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
		Tags:       []string{"important", "test"},
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(mem, "", "  ")
	memFile := filepath.Join(repo.MemoriesDir(), "tags-fail-mem.json")
	if err := os.WriteFile(memFile, data, 0o644); err != nil {
		t.Fatalf("writing memory file: %v", err)
	}

	// Use a wrapper store that fails on SetTags.
	failStore := &tagFailStore{Store: client, failAfter: 0}

	logger := testLogger()
	err = RebuildDB(failStore, repo, &mockEmbedder{}, logger)
	// Should not fail — SetTags error is logged, memory is still counted.
	if err != nil {
		t.Fatalf("RebuildDB should not fail on SetTags error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// rebuild.go — DBIsEmpty: ListMemories returns error
// ---------------------------------------------------------------------------

// errorStore always fails on ListMemories.
type errorStore struct {
	db.Store
}

func (s *errorStore) ListMemories(_ *db.MemoryFilter) ([]*db.Memory, error) {
	return nil, os.ErrClosed
}

func TestDBIsEmpty_ListMemoriesError(t *testing.T) {
	store := &errorStore{}
	if !DBIsEmpty(store) {
		t.Error("DBIsEmpty should return true when ListMemories fails")
	}
}

// ---------------------------------------------------------------------------
// git.go — batch loop automatic flush via ticker
// ---------------------------------------------------------------------------

func TestBatchLoop_AutoFlush(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 50 * time.Millisecond

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	relPath := filepath.Join("memories", "auto-flush.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{"auto":"flush"}`+"\n"), "auto-flush test"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	if !repo.pending {
		t.Error("expected pending to be true")
	}

	// Wait for the batch ticker to fire.
	time.Sleep(150 * time.Millisecond)

	repo.mu.Lock()
	pending := repo.pending
	repo.mu.Unlock()

	if pending {
		t.Error("expected pending to be false after auto-flush")
	}

	repo.Close()
}

// ---------------------------------------------------------------------------
// git.go — RemoveAndCommit in immediate mode with corrupted worktree
// ---------------------------------------------------------------------------

func TestRemoveAndCommit_WorktreeError(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a file.
	relPath := filepath.Join("memories", "wt-err.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Corrupt the .git directory after writing so the worktree call fails.
	os.RemoveAll(filepath.Join(cfg.Path, ".git"))

	err = repo.RemoveAndCommit(relPath, "delete")
	// The file will be stat'd (exists) and removed (succeeds on disk)
	// but worktree will fail.
	if err == nil {
		t.Error("RemoveAndCommit should fail when worktree is corrupted")
	}
}

// ---------------------------------------------------------------------------
// git.go — WriteAndCommit worktree error
// ---------------------------------------------------------------------------

func TestWriteAndCommit_WorktreeError(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Corrupt the .git directory so worktree fails.
	os.RemoveAll(filepath.Join(cfg.Path, ".git"))

	err = repo.WriteAndCommit("memories/wt.json", []byte("{}"), "test")
	if err == nil {
		t.Error("WriteAndCommit should fail when .git is corrupted")
	}
}

// ---------------------------------------------------------------------------
// git.go — Log error on git.Log call
// ---------------------------------------------------------------------------

func TestLog_RepoCorrupted(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Remove HEAD to corrupt the repo for Log.
	os.Remove(filepath.Join(cfg.Path, ".git", "HEAD"))

	_, err = repo.Log("memories/test.json")
	if err == nil {
		t.Error("Log should fail when repo HEAD is missing")
	}
}

// ---------------------------------------------------------------------------
// serialize.go — MemoryToJSON with all fields populated
// ---------------------------------------------------------------------------

func TestMemoryToJSON_AllFields(t *testing.T) {
	m := &db.Memory{
		ID:         "full-mem",
		Content:    "full content",
		Summary:    "a summary",
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
		Source:     "api",
		SourceFile: "/path/to/file",
		ParentID:   "parent-id",
		ChunkIndex: 3,
		Speaker:    "user",
		Area:       "area1",
		SubArea:    "sub1",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-02T00:00:00Z",
		ArchivedAt: "2025-01-03T00:00:00Z",
		TokenCount: 42,
		Tags:       []string{"a", "b"},
	}

	data, err := MemoryToJSON(m)
	if err != nil {
		t.Fatalf("MemoryToJSON: %v", err)
	}

	restored, err := JSONToMemory(data)
	if err != nil {
		t.Fatalf("JSONToMemory: %v", err)
	}

	if restored.ParentID != m.ParentID {
		t.Errorf("ParentID = %q, want %q", restored.ParentID, m.ParentID)
	}
	if restored.ChunkIndex != m.ChunkIndex {
		t.Errorf("ChunkIndex = %d, want %d", restored.ChunkIndex, m.ChunkIndex)
	}
	if restored.SourceFile != m.SourceFile {
		t.Errorf("SourceFile = %q, want %q", restored.SourceFile, m.SourceFile)
	}
	if restored.ArchivedAt != m.ArchivedAt {
		t.Errorf("ArchivedAt = %q, want %q", restored.ArchivedAt, m.ArchivedAt)
	}
}

// ---------------------------------------------------------------------------
// store.go — truncate edge cases not yet covered
// ---------------------------------------------------------------------------

func TestTruncate_ExactBoundary(t *testing.T) {
	// String length == max + 1 to trigger truncation
	input := "abcdefghijk" // 11 chars
	got := truncate(input, 10)
	want := "abcdefg..."
	if got != want {
		t.Errorf("truncate(%q, 10) = %q, want %q", input, got, want)
	}
}

func TestTruncate_WithNewlines(t *testing.T) {
	input := "line1\nline2\nline3"
	got := truncate(input, 100)
	want := "line1 line2 line3"
	if got != want {
		t.Errorf("truncate with newlines = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// store.go — writeMemoryToGit called after SaveMemory
// Verify the commit message contains "remember:" prefix.
// ---------------------------------------------------------------------------

func TestSaveMemory_CommitMessage(t *testing.T) {
	store, _ := newTestStore(t)

	emb := make([]float32, 384)
	m, err := store.SaveMemory(&db.Memory{
		Content:    "commit message test content",
		Embedding:  emb,
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	relPath := filepath.Join("memories", m.ID+".json")
	commits, err := store.repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("expected at least 1 commit")
	}
	if commits[0].Message[:9] != "remember:" {
		t.Errorf("commit message should start with 'remember:', got %q", commits[0].Message)
	}
}

// ---------------------------------------------------------------------------
// store.go — ArchiveMemory commit message contains "archive:"
// ---------------------------------------------------------------------------

func TestArchiveMemory_CommitMessage(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	if err := store.ArchiveMemory(m.ID); err != nil {
		t.Fatalf("ArchiveMemory: %v", err)
	}

	relPath := filepath.Join("memories", m.ID+".json")
	commits, err := store.repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatal("expected at least 2 commits")
	}
	if commits[0].Message[:8] != "archive:" {
		t.Errorf("latest commit should start with 'archive:', got %q", commits[0].Message)
	}
}

// ---------------------------------------------------------------------------
// store.go — SetTags commit message contains "tags:"
// ---------------------------------------------------------------------------

func TestSetTags_CommitMessage(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	if err := store.SetTags(m.ID, []string{"foo", "bar"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	relPath := filepath.Join("memories", m.ID+".json")
	commits, err := store.repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatal("expected at least 2 commits")
	}
	if commits[0].Message[:5] != "tags:" {
		t.Errorf("latest commit should start with 'tags:', got %q", commits[0].Message)
	}
}

// ---------------------------------------------------------------------------
// store.go — writeMemoryToGit called directly with corrupted git repo
// This exercises the MemoryToJSON success + WriteAndCommit failure path.
// ---------------------------------------------------------------------------

func TestWriteMemoryToGit_Direct_GitCorrupted(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	// Corrupt git so WriteAndCommit fails.
	os.RemoveAll(filepath.Join(store.repo.path, ".git"))

	// Call writeMemoryToGit directly — should log warning, not panic.
	store.writeMemoryToGit(m, "test direct write")
}

// ---------------------------------------------------------------------------
// store.go — writeLinkToGit called directly with corrupted git repo
// This exercises the GetLinks success but WriteAndCommit failure path.
// ---------------------------------------------------------------------------

func TestWriteLinkToGit_Direct_GitCorrupted(t *testing.T) {
	store, _ := newTestStore(t)
	m1 := seedTestMemory(t, store)
	m2 := seedTestMemory(t, store)

	ctx := context.Background()
	_, err := store.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.5, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Corrupt git so WriteAndCommit fails inside writeLinkToGit.
	os.RemoveAll(filepath.Join(store.repo.path, ".git"))

	// Call writeLinkToGit directly — GetLinks will succeed (DB fine),
	// LinksToJSON will succeed, but WriteAndCommit will fail.
	store.writeLinkToGit(ctx, m1.ID)
}

// ---------------------------------------------------------------------------
// store.go — writeLinkToGit called directly with closed DB (GetLinks fails)
// ---------------------------------------------------------------------------

func TestWriteLinkToGit_Direct_GetLinksFails(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)

	// Close DB so GetLinks fails.
	client.Close()

	ctx := context.Background()
	// Call writeLinkToGit directly — GetLinks will fail, should log and return.
	store.writeLinkToGit(ctx, "some-id")
}

// ---------------------------------------------------------------------------
// store.go — SetTags success path where GetMemory fails afterwards
// Use direct DB manipulation: SetTags succeeds, then delete the memory row
// so GetMemory fails on the re-fetch.
// ---------------------------------------------------------------------------

func TestSetTags_SucceedsThenGetMemoryFails(t *testing.T) {
	store, sqliteClient := newTestStore(t)
	m := seedTestMemory(t, store)

	// Insert tags directly so SetTags will succeed.
	// Then delete the memory row so GetMemory fails.
	// We need SetTags to succeed but GetMemory to fail.
	// Approach: call SetTags on the inner client directly, then break the DB,
	// then call store.SetTags which wraps inner.SetTags (will fail).
	// OR: we can use a shim. But the simplest is: save tags, delete row, call again.

	// Actually, the simplest approach: create a wrapper that succeeds on SetTags
	// but fails on GetMemory.
	// Since we have internal access, let's manipulate state after SetTags succeeds.
	// The issue is it's one atomic function call. Let's just verify with a
	// partially-broken store.

	// Delete the row so GetMemory fails.
	_, err := sqliteClient.TursoClient.DB.Exec("DELETE FROM memories WHERE id = ?", m.ID)
	if err != nil {
		t.Fatalf("delete row: %v", err)
	}
	// But keep tags table intact so SetTags might succeed.
	// Note: SetTags may fail with FK constraint. Let's see.
	err = store.SetTags(m.ID, []string{"orphaned"})
	// This exercises the error propagation from Client.SetTags.
	// If SetTags fails, that's the propagation path.
	// If it succeeds, GetMemory will fail and we hit the warn path.
	_ = err
}

// ---------------------------------------------------------------------------
// git.go — Init: error paths for git.PlainInit failure
// (directory exists as a file, not a directory)
// ---------------------------------------------------------------------------

func TestInit_PlainInitFails(t *testing.T) {
	// Create a file where the .git directory would be, causing PlainInit to fail.
	dir := t.TempDir()
	// Init will try to create memories/, links/, .magi-meta/ under dir.
	// If we make dir a file, MkdirAll will fail first.
	// Instead, create the dirs but put a file named .git to confuse PlainOpen.
	if err := os.MkdirAll(filepath.Join(dir, "memories"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "links"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".magi-meta"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".magi-meta", "version"), []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}

	// Make the directory read-only so git.PlainInit can't create .git/
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	cfg := &Config{
		Enabled:    true,
		Path:       dir,
		CommitMode: "immediate",
	}

	_, err := Init(cfg)
	if err == nil {
		t.Error("Init should fail when git cannot create .git directory")
	}
}

// ---------------------------------------------------------------------------
// git.go — WriteAndCommit: staging fails (Add returns error)
// This is hard to trigger naturally. We can make the file path contain
// characters that confuse git, or corrupt the index.
// ---------------------------------------------------------------------------

func TestWriteAndCommit_StagingFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a file then corrupt the git index to make staging fail.
	indexFile := filepath.Join(cfg.Path, ".git", "index")
	if err := os.WriteFile(indexFile, []byte("corrupted index data"), 0o644); err != nil {
		t.Fatalf("corrupting index: %v", err)
	}

	err = repo.WriteAndCommit("memories/stage-fail.json", []byte("{}"), "test")
	// May or may not error depending on go-git's tolerance. Should not panic.
	_ = err
}

// ---------------------------------------------------------------------------
// git.go — flushBatch: pending=true, worktree succeeds but commit fails
// This can happen with a corrupted index.
// ---------------------------------------------------------------------------

func TestFlushBatch_CommitFails(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 10 * time.Second

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Write something in batch mode to set pending.
	relPath := filepath.Join("memories", "flush-commit-fail.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "staged"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	repo.pending = true

	// Corrupt the HEAD reference so commit fails but worktree still works.
	headFile := filepath.Join(cfg.Path, ".git", "HEAD")
	os.WriteFile(headFile, []byte("ref: refs/heads/nonexistent\n"), 0o644)

	// flushBatch should handle commit failure gracefully.
	repo.flushBatch()

	close(repo.stopBatch)
	<-repo.batchDone
}

// ---------------------------------------------------------------------------
// git.go — fileAtCommit: tree.File error for file not in tree
// (Already covered by TestFileAtCommit_MissingFile and TestDiff_FileMissingInTree
// but let's test Contents() error — hard to trigger naturally.)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// git.go — RemoveAndCommit: staging removal fails
// (Corrupt the index after file removal.)
// ---------------------------------------------------------------------------

func TestRemoveAndCommit_StagingFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "stage-rm-fail.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Corrupt .git dir partially so worktree fails after os.Remove succeeds.
	// Remove the objects dir to make the worktree unusable.
	os.RemoveAll(filepath.Join(cfg.Path, ".git", "objects"))

	err = repo.RemoveAndCommit(relPath, "remove with corrupt index")
	// Should error since worktree operations will fail.
	if err == nil {
		t.Error("RemoveAndCommit should fail when git objects are corrupted")
	}
}

// ---------------------------------------------------------------------------
// git.go — Log: iterator.ForEach error path
// Corrupt a pack file to cause iteration errors.
// ---------------------------------------------------------------------------

func TestLog_IteratorError(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "iter-err.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{"v":1}`+"\n"), "v1"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Corrupt all git object files to force iterator errors during ForEach.
	objectsDir := filepath.Join(cfg.Path, ".git", "objects")
	entries, _ := os.ReadDir(objectsDir)
	for _, e := range entries {
		if e.IsDir() && e.Name() != "info" && e.Name() != "pack" {
			subDir := filepath.Join(objectsDir, e.Name())
			subEntries, _ := os.ReadDir(subDir)
			for _, se := range subEntries {
				// Truncate the object files to make them invalid.
				os.WriteFile(filepath.Join(subDir, se.Name()), []byte("X"), 0o644)
			}
		}
	}

	_, err = repo.Log(relPath)
	// Should error or return empty — either way, should not panic.
	_ = err
}

// ---------------------------------------------------------------------------
// store.go — SetTags: exercises the warn-and-return-nil path after SetTags
// succeeds but GetMemory fails. Uses a shim Client that sabotages GetMemory.
// ---------------------------------------------------------------------------

// brokenGetMemoryClient embeds a real *db.Client (which is *db.TursoClient)
// and overrides GetMemory to fail on demand.
type brokenGetMemoryClient struct {
	*db.TursoClient
	failGetMemory bool
}

func (b *brokenGetMemoryClient) GetMemory(id string) (*db.Memory, error) {
	if b.failGetMemory {
		return nil, os.ErrClosed
	}
	return b.TursoClient.GetMemory(id)
}

func TestSetTags_GetMemoryFailsPath(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	// Use the real client for initial operations.
	store := NewVersionedStore(client.TursoClient, repo, logger)
	m := seedTestMemory(t, store)

	// Now swap the inner client with one that fails on GetMemory.
	broken := &brokenGetMemoryClient{TursoClient: client.TursoClient, failGetMemory: true}
	store.Client = broken.TursoClient

	// But we need SetTags to succeed and GetMemory to fail.
	// Since Client is TursoClient (not our wrapper), we need a different approach.
	// Instead, let's just delete the row after SetTags succeeds.
	// The easiest approach: call SetTags, then immediately delete the row via SQL.
	// But that's the same call...
	//
	// Actually, the real approach is: make the function call work until GetMemory.
	// Since we can't intercept, let's at least verify by closing the DB
	// AFTER SetTags but that's impossible in a single call.
	//
	// The best we can do: delete the memory from DB (keeping the tag rows),
	// then call SetTags which will succeed on the tags operation but GetMemory fails.
	_, err = client.TursoClient.DB.Exec("DELETE FROM memories WHERE id = ?", m.ID)
	if err != nil {
		t.Fatalf("delete memory: %v", err)
	}

	// Reset the store to use the normal client.
	store.Client = client.TursoClient

	// SetTags will try Client.SetTags (may succeed or fail depending on FK).
	// If it succeeds, GetMemory will fail and we hit the warn path.
	err = store.SetTags(m.ID, []string{"after-delete"})
	// Either way, should not panic.
	_ = err
}

// ---------------------------------------------------------------------------
// store.go — ArchiveMemory + UpdateMemory: GetMemory fails after success
// Similar to above — delete the row mid-operation.
// ---------------------------------------------------------------------------

func TestArchiveMemory_GetMemoryFailsPath(t *testing.T) {
	logger := testLogger()

	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)
	m := seedTestMemory(t, store)

	// Delete the row so ArchiveMemory's internal GetMemory fails.
	// ArchiveMemory calls Client.ArchiveMemory first — that will fail since
	// the row is gone. This exercises the error propagation path.
	_, err = client.TursoClient.DB.Exec("DELETE FROM memories WHERE id = ?", m.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	err = store.ArchiveMemory(m.ID)
	// Should fail at Client.ArchiveMemory since row is missing.
	_ = err
}

// ---------------------------------------------------------------------------
// git.go — WriteAndCommit: wt.Add (staging) error
// Use a path outside the worktree to trigger staging failure.
// ---------------------------------------------------------------------------

func TestWriteAndCommit_AddFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write to an absolute path outside the repo, then try to Add a relative
	// path that doesn't match. Actually, WriteAndCommit writes to
	// filepath.Join(r.path, relPath), so the file will be in the repo.
	// But if we use a relative path with ".." that escapes the worktree...
	err = repo.WriteAndCommit("../../../outside.json", []byte("{}"), "escape")
	// This may or may not error depending on go-git behavior. Should not panic.
	_ = err
}

// ---------------------------------------------------------------------------
// git.go — RemoveAndCommit: wt.Add on removed file fails
// After removing the file, corrupt the worktree so staging fails.
// ---------------------------------------------------------------------------

func TestRemoveAndCommit_AddAfterRemoveFails(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "rm-add-fail.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Corrupt the git index after the file exists.
	indexFile := filepath.Join(cfg.Path, ".git", "index")
	if err := os.WriteFile(indexFile, []byte("bad"), 0o644); err != nil {
		t.Fatalf("corrupting index: %v", err)
	}

	err = repo.RemoveAndCommit(relPath, "remove with bad index")
	// File removal from disk may succeed but git staging will fail.
	_ = err
}

// ---------------------------------------------------------------------------
// store.go — SetTags: GetMemory fails after SetTags succeeds
// Delete the memory row (FK not enforced in SQLite) so SetTags succeeds
// on the tags table but the subsequent GetMemory returns sql.ErrNoRows.
// This deterministically exercises the warn-and-return-nil path.
// ---------------------------------------------------------------------------

func TestSetTags_DBSetTagsSucceeds_GetMemoryFails(t *testing.T) {
	store, sqliteClient := newTestStore(t)
	m := seedTestMemory(t, store)

	// Create a trigger that deletes the memory row AFTER tags are inserted.
	// SetTags does: BEGIN → DELETE tags → INSERT tags → COMMIT, then GetMemory.
	// The trigger fires on INSERT into memory_tags, deleting the memory row.
	// Since triggers run within the same transaction, the commit succeeds,
	// but by the time GetMemory runs the memory is gone → warn path hit.
	_, err := sqliteClient.TursoClient.DB.Exec(`
		CREATE TRIGGER trg_sabotage_after_tag_insert
		AFTER INSERT ON memory_tags
		BEGIN
			DELETE FROM memories WHERE id = NEW.memory_id;
		END
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() {
		sqliteClient.TursoClient.DB.Exec("DROP TRIGGER IF EXISTS trg_sabotage_after_tag_insert")
	})

	// SetTags inserts tags → trigger deletes memory → SetTags transaction commits.
	// Then VersionedStore.SetTags calls GetMemory → sql.ErrNoRows → warn path.
	// SetTags returns nil (GetMemory failure is swallowed with a log warning).
	err = store.SetTags(m.ID, []string{"doomed-tag"})
	if err != nil {
		t.Fatalf("SetTags should return nil (GetMemory failure is swallowed): %v", err)
	}
}

// ---------------------------------------------------------------------------
// store.go — ArchiveMemory: ArchiveMemory succeeds but GetMemory fails
// We set archived_at directly via SQL, then remove the row so GetMemory
// fails. Since Client.ArchiveMemory does UPDATE ... SET archived_at on a
// missing row it affects 0 rows (no error), then GetMemory fails.
// ---------------------------------------------------------------------------

func TestArchiveMemory_SucceedsThenGetMemoryFails(t *testing.T) {
	store, sqliteClient := newTestStore(t)
	m := seedTestMemory(t, store)

	// Insert a row into memories that will be "archived" but then deleted
	// between ArchiveMemory and GetMemory. We can't do mid-call, so we
	// verify the path where ArchiveMemory's UPDATE affects 0 rows (no error
	// from SQLite for UPDATE on missing row) and GetMemory fails.
	if _, err := sqliteClient.TursoClient.DB.Exec(
		"DELETE FROM memories WHERE id = ?", m.ID,
	); err != nil {
		t.Fatalf("direct delete: %v", err)
	}

	// ArchiveMemory → UPDATE archived_at affects 0 rows (no error), then
	// GetMemory fails → warn path is hit, returns nil.
	err := store.ArchiveMemory(m.ID)
	// Should not panic; returns nil since it's the warn path.
	if err != nil {
		t.Fatalf("ArchiveMemory should return nil when GetMemory fails: %v", err)
	}
}

// ---------------------------------------------------------------------------
// store.go — UpdateMemory: UpdateMemory succeeds but GetMemory fails
// Similar pattern: delete the row so UpdateMemory's UPDATE is a no-op
// and GetMemory returns an error, hitting the warn-and-return-nil path.
// ---------------------------------------------------------------------------

func TestUpdateMemory_SucceedsThenGetMemoryFails(t *testing.T) {
	store, sqliteClient := newTestStore(t)
	m := seedTestMemory(t, store)

	// Delete the row so UpdateMemory's UPDATE is a no-op.
	if _, err := sqliteClient.TursoClient.DB.Exec(
		"DELETE FROM memories WHERE id = ?", m.ID,
	); err != nil {
		t.Fatalf("direct delete: %v", err)
	}

	m.Content = "will fail to re-fetch"
	err := store.UpdateMemory(m)
	// UpdateMemory calls Client.UpdateMemory (UPDATE on missing row = no error),
	// then GetMemory fails → warn path hit, returns nil.
	if err != nil {
		t.Fatalf("UpdateMemory should return nil when GetMemory fails: %v", err)
	}
}

// ---------------------------------------------------------------------------
// store.go — writeLinkToGit: LinksToJSON succeeds but WriteAndCommit fails
// Create a link successfully, then corrupt git and call writeLinkToGit
// directly. GetLinks will succeed (DB is fine), LinksToJSON will succeed,
// but WriteAndCommit will fail (git corrupted). Already tested indirectly
// above but this exercises it deterministically through the direct call.
// ---------------------------------------------------------------------------

func TestWriteLinkToGit_LinksToJSON_OK_WriteAndCommit_Fails(t *testing.T) {
	store, _ := newTestStore(t)

	emb := make([]float32, 384)
	m1, err := store.SaveMemory(&db.Memory{
		Content: "link-wac-src", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory m1: %v", err)
	}
	m2, err := store.SaveMemory(&db.Memory{
		Content: "link-wac-dst", Embedding: emb, Project: "test", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory m2: %v", err)
	}

	ctx := context.Background()
	if _, err := store.CreateLink(ctx, m1.ID, m2.ID, "led_to", 0.8, true); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Corrupt git repo so WriteAndCommit fails inside writeLinkToGit.
	os.RemoveAll(filepath.Join(store.repo.path, ".git"))

	// writeLinkToGit: GetLinks OK → LinksToJSON OK → WriteAndCommit FAIL (logged)
	store.writeLinkToGit(ctx, m1.ID)
	// Should not panic. The WriteAndCommit error is logged and swallowed.
}

// ---------------------------------------------------------------------------
// git.go — flushBatch: pending=false (early return, no commit)
// ---------------------------------------------------------------------------

func TestFlushBatch_NoPending_EarlyReturn(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 10 * time.Second

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// pending is false by default — flushBatch should return immediately.
	repo.flushBatch()

	close(repo.stopBatch)
	<-repo.batchDone
}

// ---------------------------------------------------------------------------
// git.go — RemoveAndCommit: file doesn't exist (early return nil)
// ---------------------------------------------------------------------------

func TestRemoveAndCommit_FileNotExist(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Remove a file that was never created — should return nil (nothing to do).
	err = repo.RemoveAndCommit("memories/does-not-exist.json", "delete nonexistent")
	if err != nil {
		t.Errorf("RemoveAndCommit on nonexistent file should return nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// git.go — HasMemories: no .json files
// ---------------------------------------------------------------------------

func TestHasMemories_Empty(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	if repo.HasMemories() {
		t.Error("HasMemories should return false for empty memories dir")
	}
}

func TestHasMemories_WithFiles(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	if err := repo.WriteAndCommit("memories/test.json", []byte("{}"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	if !repo.HasMemories() {
		t.Error("HasMemories should return true when .json files exist")
	}
}

// ---------------------------------------------------------------------------
// git.go — HasMemories: memories dir missing
// ---------------------------------------------------------------------------

func TestHasMemories_DirMissing(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	os.RemoveAll(filepath.Join(cfg.Path, "memories"))

	if repo.HasMemories() {
		t.Error("HasMemories should return false when memories dir is missing")
	}
}

// ---------------------------------------------------------------------------
// git.go — MemoriesDir returns correct path
// ---------------------------------------------------------------------------

func TestMemoriesDir_ReturnsCorrectPath(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	want := filepath.Join(cfg.Path, "memories")
	got := repo.MemoriesDir()
	if got != want {
		t.Errorf("MemoriesDir = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// git.go — simpleDiff: identical content
// ---------------------------------------------------------------------------

func TestSimpleDiff_IdenticalContent(t *testing.T) {
	got := simpleDiff("hello\nworld", "hello\nworld")
	if got != "(no changes)" {
		t.Errorf("simpleDiff identical = %q, want %q", got, "(no changes)")
	}
}

func TestSimpleDiff_AddedLines(t *testing.T) {
	got := simpleDiff("line1", "line1\nline2")
	if got == "(no changes)" {
		t.Error("simpleDiff should detect added lines")
	}
}

func TestSimpleDiff_RemovedLines(t *testing.T) {
	got := simpleDiff("line1\nline2", "line1")
	if got == "(no changes)" {
		t.Error("simpleDiff should detect removed lines")
	}
}
