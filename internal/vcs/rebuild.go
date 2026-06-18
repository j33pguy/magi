package vcs

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// RebuildDB repopulates the database from git-backed memory files.
// Called on startup when git has memories but DB is empty.
func RebuildDB(store db.Store, repo *Repo, embedder embeddings.Provider, logger *slog.Logger) error {
	client, ok := store.(*db.Client)
	if !ok {
		return fmt.Errorf("git rebuild requires concrete sqlite/turso client")
	}

	ctx := context.Background()
	memoriesDir := repo.MemoriesDir()
	entries, err := os.ReadDir(memoriesDir)
	if err != nil {
		return fmt.Errorf("reading memories directory: %w", err)
	}

	var restoredMemories, restoredLinks, restoredContexts int
	knownIDs := make(map[string]struct{})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memoriesDir, entry.Name()))
		if err != nil {
			logger.Warn("rebuild: failed to read memory file", "file", entry.Name(), "error", err)
			continue
		}
		mem, err := JSONToMemory(data)
		if err != nil {
			logger.Warn("rebuild: failed to parse memory file", "file", entry.Name(), "error", err)
			continue
		}
		embedding, err := embedder.Embed(ctx, mem.Content)
		if err != nil {
			logger.Warn("rebuild: failed to generate embedding", "id", mem.ID, "error", err)
			continue
		}
		mem.Embedding = embedding
		tags := append([]string(nil), mem.Tags...)
		mem.Tags = nil
		if err := insertMemoryWithID(client, mem); err != nil {
			logger.Warn("rebuild: failed to insert memory", "id", mem.ID, "error", err)
			continue
		}
		if len(tags) > 0 {
			if err := client.SetTags(mem.ID, tags); err != nil {
				logger.Warn("rebuild: failed to set tags", "id", mem.ID, "error", err)
			}
		}
		knownIDs[mem.ID] = struct{}{}
		restoredMemories++
	}

	contextsDir := filepath.Join(repo.path, "contexts")
	if entries, err := os.ReadDir(contextsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(contextsDir, entry.Name()))
			if err != nil {
				logger.Warn("rebuild: failed to read context file", "file", entry.Name(), "error", err)
				continue
			}
			record, err := JSONToContext(data)
			if err != nil {
				logger.Warn("rebuild: failed to parse context file", "file", entry.Name(), "error", err)
				continue
			}
			if _, ok := knownIDs[record.MemoryID]; !ok {
				continue
			}
			if err := client.SaveMemoryContext(record); err != nil {
				logger.Warn("rebuild: failed to restore memory context", "memory_id", record.MemoryID, "error", err)
				continue
			}
			restoredContexts++
		}
	}

	linksDir := filepath.Join(repo.path, "links")
	if entries, err := os.ReadDir(linksDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			fromID := strings.TrimSuffix(entry.Name(), ".json")
			if _, ok := knownIDs[fromID]; !ok {
				continue
			}
			data, err := os.ReadFile(filepath.Join(linksDir, entry.Name()))
			if err != nil {
				logger.Warn("rebuild: failed to read link file", "file", entry.Name(), "error", err)
				continue
			}
			links, err := JSONToLinks(fromID, data)
			if err != nil {
				logger.Warn("rebuild: failed to parse link file", "file", entry.Name(), "error", err)
				continue
			}
			for _, link := range links {
				if _, ok := knownIDs[link.ToID]; !ok {
					logger.Warn("rebuild: skipping link with missing endpoint", "link_id", link.ID, "from_id", link.FromID, "to_id", link.ToID)
					continue
				}
				if err := insertLinkWithID(client, link); err != nil {
					logger.Warn("rebuild: failed to restore link", "link_id", link.ID, "from_id", link.FromID, "to_id", link.ToID, "error", err)
					continue
				}
				restoredLinks++
			}
		}
	}

	logger.Info("rebuild: completed", "memories_restored", restoredMemories, "contexts_restored", restoredContexts, "links_restored", restoredLinks)
	return nil
}

func insertMemoryWithID(client *db.Client, mem *db.Memory) error {
	visibility := mem.Visibility
	if visibility == "" {
		visibility = "internal"
	}
	_, err := client.DB.Exec(`
		INSERT INTO memories (id, content, summary, embedding, project, type, visibility, source, source_file, parent_id, chunk_index, speaker, area, sub_area, created_at, updated_at, archived_at, token_count)
		VALUES (?, ?, ?, vector32(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		mem.ID,
		mem.Content,
		nullString(mem.Summary),
		float32sToBytes(mem.Embedding),
		mem.Project,
		mem.Type,
		visibility,
		nullString(mem.Source),
		nullString(mem.SourceFile),
		nullString(mem.ParentID),
		mem.ChunkIndex,
		mem.Speaker,
		mem.Area,
		mem.SubArea,
		mem.CreatedAt,
		mem.UpdatedAt,
		nullString(mem.ArchivedAt),
		mem.TokenCount,
	)
	if err != nil {
		return fmt.Errorf("inserting memory with preserved id: %w", err)
	}
	return nil
}

func insertLinkWithID(client *db.Client, link *db.MemoryLink) error {
	autoInt := 0
	if link.Auto {
		autoInt = 1
	}
	_, err := client.DB.ExecContext(context.Background(), `
		INSERT INTO memory_links (id, from_id, to_id, relation, weight, auto, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, link.ID, link.FromID, link.ToID, link.Relation, link.Weight, autoInt, link.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting link with preserved id: %w", err)
	}
	return nil
}

func float32sToBytes(v []float32) []byte {
	if v == nil {
		return nil
	}
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// DBIsEmpty checks if the database has any memories.
func DBIsEmpty(store db.Store) bool {
	memories, err := store.ListMemories(&db.MemoryFilter{Limit: 1, Visibility: "all"})
	if err != nil {
		return true
	}
	return len(memories) == 0
}
