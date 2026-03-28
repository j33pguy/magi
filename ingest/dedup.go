package ingest

import (
	"context"
	"log/slog"
)

// Embedder generates vector embeddings for text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// DBClient provides deduplication-related database queries.
type DBClient interface {
	ExistsWithContentHash(hash string) (string, error)
}

// Deduplicator checks new memories against existing ones to prevent duplicates.
type Deduplicator struct {
	DB       DBClient
	Embedder Embedder
}

// dedupThreshold is the cosine similarity above which memories are considered duplicates.
const dedupThreshold = 0.92

// Filter removes memories from candidates that are too similar to existing ones.
// Uses content-hash dedup (exact match via SHA-256 prefix).
func (d *Deduplicator) Filter(ctx context.Context, candidates []ExtractedMemory) (kept []ExtractedMemory, skipped int, err error) {
	for _, c := range candidates {
		hash := contentHash(c.Content)
		existingID, err := d.DB.ExistsWithContentHash(hash)
		if err != nil {
			slog.Warn("dedup hash check failed, keeping candidate", "error", err)
			kept = append(kept, c)
			continue
		}
		if existingID != "" {
			skipped++
			continue
		}
		kept = append(kept, c)
	}
	return kept, skipped, nil
}
