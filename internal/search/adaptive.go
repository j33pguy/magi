// Package search provides adaptive retrieval with document grading and query rewriting.
package search

import (
	"context"
	"fmt"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/rewrite"
)

// Response wraps search results with adaptive retrieval metadata.
type Response struct {
	Results        []*db.HybridResult `json:"results"`
	Rewritten      bool               `json:"rewritten"`
	RewrittenQuery string             `json:"rewritten_query,omitempty"`
	Attempts       int                `json:"attempts"`
}

// EmbedFunc generates a vector embedding for the given text.
type EmbedFunc func(ctx context.Context, text string) ([]float32, error)

// Adaptive performs hybrid search with document grading and query rewriting.
// If no results pass the min_relevance threshold (or zero results are returned),
// the query is rewritten deterministically and retried once.
// Set minRelevance to 0 to disable grading (all results pass).
// Set recencyDecay > 0 to apply exponential recency weighting (recommended: 0.01).
func Adaptive(ctx context.Context, client *db.Client, embed EmbedFunc, query string, filter *db.MemoryFilter, topK int, minRelevance float64, recencyDecay float64) (*Response, error) {
	embedding, err := embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generating embedding: %w", err)
	}

	results, err := client.HybridSearch(embedding, query, filter, topK)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	resolveParents(client, results)
	filtered := gradeResults(results, minRelevance)
	ApplyRecencyWeighting(filtered, recencyDecay)

	resp := &Response{
		Results:  filtered,
		Attempts: 1,
	}

	// Retry with rewritten query if no results pass
	if len(filtered) == 0 {
		rewritten := rewrite.Query(query)
		if rewritten != query && rewritten != "" {
			embedding2, err := embed(ctx, rewritten)
			if err != nil {
				return resp, nil
			}

			results2, err := client.HybridSearch(embedding2, rewritten, filter, topK)
			if err != nil {
				return resp, nil
			}

			resolveParents(client, results2)

			graded := gradeResults(results2, minRelevance)
			ApplyRecencyWeighting(graded, recencyDecay)
			resp.Results = graded
			resp.Rewritten = true
			resp.RewrittenQuery = rewritten
			resp.Attempts = 2
		}
	}

	return resp, nil
}

// gradeResults filters results by minimum relevance score.
// A minRelevance of 0 disables filtering (returns all results).
func gradeResults(results []*db.HybridResult, minRelevance float64) []*db.HybridResult {
	if minRelevance <= 0 {
		return results
	}

	filtered := make([]*db.HybridResult, 0, len(results))
	for _, r := range results {
		if r.Score >= minRelevance {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// resolveParents replaces chunk content with parent document content.
func resolveParents(client *db.Client, results []*db.HybridResult) {
	for _, result := range results {
		if result.Memory.ParentID != "" {
			parent, err := client.GetMemory(result.Memory.ParentID)
			if err == nil {
				result.Memory.Content = parent.Content
				result.Memory.Tags = parent.Tags
			}
		}
	}
}
