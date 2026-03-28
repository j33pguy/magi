package patterns

import (
	"context"
	"fmt"
	"strings"

	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
)

// StorePatterns saves detected patterns as memories in the DB.
// Each pattern becomes a memory with speaker="system", importance=4, and
// appropriate tags. Deduplicates by checking if a memory with the same
// pattern_type tag and >0.9 similarity already exists.
//
// Returns IDs of newly stored patterns and count of skipped duplicates.
func StorePatterns(ctx context.Context, dbClient *db.Client, embedder embeddings.Provider, patterns []Pattern) (stored []string, skipped int, err error) {
	for _, p := range patterns {
		// Generate embedding for the pattern description
		embedding, err := embedder.Embed(ctx, p.Description)
		if err != nil {
			return stored, skipped, fmt.Errorf("embedding pattern %q: %w", p.Description, err)
		}

		// Dedup: check for existing similar pattern of same type
		// Search for memories with the pattern tag and check similarity
		filter := &db.MemoryFilter{
			Tags:       []string{"pattern_type:" + string(p.Type)},
			Visibility: "all",
		}
		results, err := dbClient.SearchMemories(embedding, filter, 1)
		if err == nil && len(results) > 0 {
			similarity := 1.0 - results[0].Distance
			if similarity > 0.9 {
				skipped++
				continue
			}
		}

		// Build tags
		tags := []string{
			"pattern",
			"auto-detected",
			"pattern_type:" + string(p.Type),
			"speaker:system",
		}
		if p.Area != "" {
			tags = append(tags, "area:"+p.Area)
		}

		// Build summary with confidence
		summary := fmt.Sprintf("[%s] %s (confidence: %.0f%%)", p.Type, truncateDesc(p.Description, 80), p.Confidence*100)

		// Build content with evidence
		content := p.Description
		if len(p.Evidence) > 0 {
			content += "\n\nEvidence memory IDs: " + strings.Join(p.Evidence, ", ")
		}

		mem := &db.Memory{
			Content:    content,
			Summary:    summary,
			Embedding:  embedding,
			Type:       "preference",
			Speaker:    "system",
			Area:       p.Area,
			Source:     "pattern-analyzer",
			Visibility: "internal",
			TokenCount: len(content) / 4,
		}

		saved, err := dbClient.SaveMemory(mem)
		if err != nil {
			return stored, skipped, fmt.Errorf("saving pattern %q: %w", p.Description, err)
		}

		if err := dbClient.SetTags(saved.ID, tags); err != nil {
			return stored, skipped, fmt.Errorf("setting tags for pattern %s: %w", saved.ID, err)
		}

		stored = append(stored, saved.ID)
	}
	return stored, skipped, nil
}

func truncateDesc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
