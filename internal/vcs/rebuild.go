package vcs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// RebuildDB repopulates the database from git memory files.
// Called on startup when git has memories but DB is empty.
func RebuildDB(store db.Store, repo *Repo, embedder embeddings.Provider, logger *slog.Logger) error {
	memoriesDir := repo.MemoriesDir()
	entries, err := os.ReadDir(memoriesDir)
	if err != nil {
		return fmt.Errorf("reading memories directory: %w", err)
	}

	ctx := context.Background()
	var count int

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(memoriesDir, entry.Name()))
		if err != nil {
			logger.Warn("rebuild: failed to read file", "file", entry.Name(), "error", err)
			continue
		}

		mem, err := JSONToMemory(data)
		if err != nil {
			logger.Warn("rebuild: failed to parse file", "file", entry.Name(), "error", err)
			continue
		}

		// Generate embedding for the memory content
		embedding, err := embedder.Embed(ctx, mem.Content)
		if err != nil {
			logger.Warn("rebuild: failed to generate embedding", "id", mem.ID, "error", err)
			continue
		}
		mem.Embedding = embedding

		// Save to DB (will generate a new ID — we need to preserve the original)
		tags := mem.Tags
		mem.Tags = nil // Tags are saved separately

		saved, err := store.SaveMemory(mem)
		if err != nil {
			logger.Warn("rebuild: failed to save memory", "id", mem.ID, "error", err)
			continue
		}

		if len(tags) > 0 {
			if err := store.SetTags(saved.ID, tags); err != nil {
				logger.Warn("rebuild: failed to set tags", "id", saved.ID, "error", err)
			}
		}

		count++
	}

	logger.Info("rebuild: completed", "memories_restored", count)
	return nil
}

// DBIsEmpty checks if the database has any memories.
func DBIsEmpty(store db.Store) bool {
	memories, err := store.ListMemories(&db.MemoryFilter{Limit: 1, Visibility: "all"})
	if err != nil {
		return true
	}
	return len(memories) == 0
}
