package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// ============================================================
// Import — uncovered branches in markdown.go
// ============================================================

// TestImport_PathTraversalDotPrefix covers the ".." prefix check (line 54).
func TestImport_PathTraversalDotPrefix(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "..secret.md", Project: "proj", Type: "context"},
	})
	if err == nil {
		t.Fatal("expected error for filename starting with '..'")
	}
}

// TestImport_ResolvedPathOutsideDir covers the resolved-path check (line 60-62).
// A filename like "." resolves to the dir itself, which does not have the
// dir + separator prefix, triggering the path-traversal guard.
func TestImport_ResolvedPathOutsideDir(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	// Filename "." resolves to dir itself, not dir + sep + something
	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: ".", Project: "proj", Type: "context"},
	})
	// "." contains no separators and doesn't start with "..", so it passes
	// the first check. But filepath.Join(dir, ".") == dir, which does not
	// have the prefix dir + "/", so it hits the path-traversal error.
	if err == nil {
		t.Fatal("expected path traversal error for filename '.'")
	}
}

// TestImport_EmptyDir covers calling Import with a relative dir that can be
// resolved by filepath.Abs successfully.
func TestImport_RelativeDir(t *testing.T) {
	imp, _ := newImporter(t)

	// Use a relative path; filepath.Abs should resolve it fine
	// All files will be missing, so they get skipped
	err := imp.Import(context.Background(), ".", []FileMapping{
		{Filename: "nonexistent_test_file_xyz.md", Project: "proj", Type: "context"},
	})
	if err != nil {
		t.Fatalf("Import with relative dir should succeed (skip missing): %v", err)
	}
}

// TestImport_MultipleFilesPartialExist covers a mix of existing and missing files.
func TestImport_MultipleFilesPartialExist(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "exists.md"), []byte("content here"), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "missing1.md", Project: "proj", Type: "context"},
		{Filename: "exists.md", Project: "proj", Type: "context", Tags: []string{"found"}},
		{Filename: "missing2.md", Project: "proj", Type: "context"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Errorf("expected 1 memory (only existing file), got %d", len(memories))
	}
}
