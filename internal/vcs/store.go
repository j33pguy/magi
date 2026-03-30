package vcs

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/j33pguy/magi/internal/db"
)

// VersionedStore wraps a db.Store and writes changes to a git repository.
// Git is secondary — if a git write fails, the DB operation still succeeds.
//
// This implements the db.Store interface so it can be used as a drop-in
// replacement anywhere a Store is expected.
type VersionedStore struct {
	*db.Client
	repo   *Repo
	logger *slog.Logger
}

// NewVersionedStore creates a middleware that wraps an existing Client with git versioning.
func NewVersionedStore(inner *db.Client, repo *Repo, logger *slog.Logger) *VersionedStore {
	return &VersionedStore{
		Client: inner,
		repo:   repo,
		logger: logger,
	}
}

// GitRepo returns the underlying git repo for history/diff queries.
func (v *VersionedStore) GitRepo() *Repo {
	return v.repo
}

// Inner returns the wrapped db.Client for consumers that need the concrete type.
func (v *VersionedStore) Inner() *db.Client {
	return v.Client
}

func (v *VersionedStore) SaveMemory(m *db.Memory) (*db.Memory, error) {
	saved, err := v.Client.SaveMemory(m)
	if err != nil {
		return nil, err
	}

	v.writeMemoryToGit(saved, fmt.Sprintf("remember: %s", truncate(saved.Content, 72)))
	return saved, nil
}

func (v *VersionedStore) UpdateMemory(m *db.Memory) error {
	if err := v.Client.UpdateMemory(m); err != nil {
		return err
	}

	// Re-fetch full memory for serialization (Update doesn't return all fields)
	full, err := v.Client.GetMemory(m.ID)
	if err != nil {
		v.logger.Warn("git: failed to fetch memory after update", "id", m.ID, "error", err)
		return nil
	}

	v.writeMemoryToGit(full, fmt.Sprintf("update: %s", truncate(full.Content, 72)))
	return nil
}

func (v *VersionedStore) DeleteMemory(id string) error {
	if err := v.Client.DeleteMemory(id); err != nil {
		return err
	}

	relPath := filepath.Join("memories", id+".json")
	if err := v.repo.RemoveAndCommit(relPath, fmt.Sprintf("delete: %s", id)); err != nil {
		v.logger.Warn("git: failed to remove memory file", "id", id, "error", err)
	}
	return nil
}

func (v *VersionedStore) ArchiveMemory(id string) error {
	if err := v.Client.ArchiveMemory(id); err != nil {
		return err
	}

	// Re-fetch to get updated archived_at
	full, err := v.Client.GetMemory(id)
	if err != nil {
		v.logger.Warn("git: failed to fetch memory after archive", "id", id, "error", err)
		return nil
	}

	v.writeMemoryToGit(full, fmt.Sprintf("archive: %s", id))
	return nil
}

func (v *VersionedStore) SetTags(memoryID string, tags []string) error {
	if err := v.Client.SetTags(memoryID, tags); err != nil {
		return err
	}

	// Re-fetch full memory with new tags
	full, err := v.Client.GetMemory(memoryID)
	if err != nil {
		v.logger.Warn("git: failed to fetch memory after tag update", "id", memoryID, "error", err)
		return nil
	}

	v.writeMemoryToGit(full, fmt.Sprintf("tags: update tags on %s", memoryID))
	return nil
}

func (v *VersionedStore) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*db.MemoryLink, error) {
	link, err := v.Client.CreateLink(ctx, fromID, toID, relation, weight, auto)
	if err != nil {
		return nil, err
	}

	v.writeLinkToGit(ctx, fromID)
	return link, nil
}

func (v *VersionedStore) DeleteLink(ctx context.Context, linkID string) error {
	if err := v.Client.DeleteLink(ctx, linkID); err != nil {
		return err
	}
	// Can't update link file without knowing from_id — acceptable tradeoff.
	// Links will be refreshed on next CreateLink for that memory.
	return nil
}

// writeMemoryToGit serializes and commits a memory file.
func (v *VersionedStore) writeMemoryToGit(m *db.Memory, message string) {
	data, err := MemoryToJSON(m)
	if err != nil {
		v.logger.Warn("git: failed to serialize memory", "id", m.ID, "error", err)
		return
	}

	relPath := filepath.Join("memories", m.ID+".json")
	if err := v.repo.WriteAndCommit(relPath, data, message); err != nil {
		v.logger.Warn("git: failed to write memory file", "id", m.ID, "error", err)
	}
}

// writeLinkToGit fetches all outbound links for a memory and writes them.
func (v *VersionedStore) writeLinkToGit(ctx context.Context, fromID string) {
	links, err := v.Client.GetLinks(ctx, fromID, "from")
	if err != nil {
		v.logger.Warn("git: failed to fetch links", "from_id", fromID, "error", err)
		return
	}

	data, err := LinksToJSON(links)
	if err != nil {
		v.logger.Warn("git: failed to serialize links", "from_id", fromID, "error", err)
		return
	}

	relPath := filepath.Join("links", fromID+".json")
	if err := v.repo.WriteAndCommit(relPath, data, fmt.Sprintf("link: update links from %s", fromID)); err != nil {
		v.logger.Warn("git: failed to write link file", "from_id", fromID, "error", err)
	}
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
