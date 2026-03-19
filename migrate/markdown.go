// Package migrate provides tools for importing existing memory files into the database.
package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/russseaman/claude-memory/chunking"
	"github.com/russseaman/claude-memory/db"
	"github.com/russseaman/claude-memory/embeddings"
)

// MarkdownImporter imports markdown memory files into the database.
type MarkdownImporter struct {
	DB       *db.Client
	Embedder embeddings.Provider
	Splitter *chunking.Splitter
	Logger   *slog.Logger
}

// FileMapping defines how a markdown file should be imported.
type FileMapping struct {
	Filename string
	Project  string
	Type     string
	Tags     []string
}

// DefaultMappings returns the import mappings for the IaC project's memory files.
func DefaultMappings() []FileMapping {
	return []FileMapping{
		{Filename: "MEMORY.md", Project: "iac", Type: "context", Tags: []string{"index"}},
		{Filename: "ephemeral-runners.md", Project: "iac", Type: "context", Tags: []string{"scaleset", "github-actions"}},
		{Filename: "security-audit-2026-03-19.md", Project: "iac", Type: "security", Tags: []string{"audit", "remediation"}},
		{Filename: "recovery-pi-project.md", Project: "iac", Type: "decision", Tags: []string{"recovery", "raspberry-pi"}},
		{Filename: "project-authentik-sso.md", Project: "iac", Type: "context", Tags: []string{"authentik", "sso"}},
		{Filename: "planned-review-session.md", Project: "iac", Type: "context", Tags: []string{"review", "tasks"}},
	}
}

// Import imports all markdown files from a directory using the provided mappings.
func (m *MarkdownImporter) Import(ctx context.Context, dir string, mappings []FileMapping) error {
	for _, mapping := range mappings {
		path := filepath.Join(dir, mapping.Filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			m.Logger.Warn("File not found, skipping", slog.String("path", path))
			continue
		}

		if err := m.importFile(ctx, path, &mapping); err != nil {
			return fmt.Errorf("importing %s: %w", mapping.Filename, err)
		}
	}

	return nil
}

func (m *MarkdownImporter) importFile(ctx context.Context, path string, mapping *FileMapping) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	text := string(content)

	// Strip frontmatter if present
	text = stripFrontmatter(text)

	chunks := m.Splitter.Split(text)

	m.Logger.Info("Importing file",
		slog.String("file", mapping.Filename),
		slog.Int("chunks", len(chunks)),
	)

	var parentID string

	for _, chunk := range chunks {
		embedding, err := m.Embedder.Embed(ctx, chunk.Content)
		if err != nil {
			return fmt.Errorf("embedding chunk %d: %w", chunk.Index, err)
		}

		memory := &db.Memory{
			Content:    chunk.Content,
			Embedding:  embedding,
			Project:    mapping.Project,
			Type:       mapping.Type,
			Source:     "import",
			SourceFile: path,
			ChunkIndex: chunk.Index,
			TokenCount: len(chunk.Content) / 4,
		}

		if chunk.Index > 0 && parentID != "" {
			memory.ParentID = parentID
		}

		saved, err := m.DB.SaveMemory(memory)
		if err != nil {
			return fmt.Errorf("saving chunk %d: %w", chunk.Index, err)
		}

		if chunk.Index == 0 {
			parentID = saved.ID
		}

		// Set tags on all chunks
		if len(mapping.Tags) > 0 {
			if err := m.DB.SetTags(saved.ID, mapping.Tags); err != nil {
				return fmt.Errorf("setting tags on chunk %d: %w", chunk.Index, err)
			}
		}
	}

	return nil
}

// stripFrontmatter removes YAML frontmatter (--- delimited) from markdown content.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// Find the closing ---
	rest := content[3:]
	idx := strings.Index(rest, "---")
	if idx < 0 {
		return content
	}

	return strings.TrimSpace(rest[idx+3:])
}
