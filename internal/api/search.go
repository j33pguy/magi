package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/rewrite"
	"github.com/j33pguy/magi/internal/search"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	query := q.Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}

	topK, _ := strconv.Atoi(q.Get("top_k"))
	if topK <= 0 {
		topK = 5
	}

	recencyDecay, _ := strconv.ParseFloat(q.Get("recency_decay"), 64)

	var tags []string
	if t := q.Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}

	embedding, err := s.embedder.Embed(r.Context(), query)
	if err != nil {
		s.logger.Error("generating query embedding", "error", err)
		s.logger.Error("generating embedding", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	filter := &db.MemoryFilter{
		Project:    q.Get("project"),
		Type:       q.Get("type"),
		Tags:       tags,
		Visibility: "", // HTTP API: exclude private memories by default
	}
	applyRequestAccessScope(r, filter)

	results, err := s.db.HybridSearch(embedding, query, filter, topK)
	if err != nil {
		s.logger.Error("hybrid search", "error", err)
		s.logger.Error("hybrid search failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	// Optional rewrite fallback: run a second retrieval pass with deterministic
	// query rewriting and merge candidates. Enable with rewrite_fallback=1.
	if q.Get("rewrite_fallback") == "1" {
		rewritten := rewrite.Query(query)
		if rewritten != "" && rewritten != query {
			rewrittenEmbedding, rerr := s.embedder.Embed(r.Context(), rewritten)
			if rerr != nil {
				s.logger.Warn("rewrite fallback embedding failed", "error", rerr)
			} else {
				extra, herr := s.db.HybridSearch(rewrittenEmbedding, rewritten, filter, topK*2)
				if herr != nil {
					s.logger.Warn("rewrite fallback search failed", "error", herr)
				} else if len(extra) > 0 {
					results = mergeHybridResults(results, extra)
					if len(results) > topK {
						results = results[:topK]
					}
				}
			}
		}
	}

	for _, result := range results {
		if result.Memory.ParentID != "" {
			parent, err := s.db.GetMemory(result.Memory.ParentID)
			if err == nil {
				if memoryAllowedForFilter(parent, parent.Tags, filter) {
					result.Memory.Content = parent.Content
					result.Memory.Tags = parent.Tags
				}
			}
		}
	}

	search.ApplyRecencyWeighting(results, recencyDecay)

	writeJSON(w, http.StatusOK, results)
}

func mergeHybridResults(primary, secondary []*db.HybridResult) []*db.HybridResult {
	merged := make([]*db.HybridResult, 0, len(primary)+len(secondary))
	seen := make(map[string]*db.HybridResult, len(primary)+len(secondary))

	for _, r := range primary {
		if r == nil || r.Memory == nil || r.Memory.ID == "" {
			continue
		}
		copyR := *r
		seen[r.Memory.ID] = &copyR
		merged = append(merged, &copyR)
	}

	for _, r := range secondary {
		if r == nil || r.Memory == nil || r.Memory.ID == "" {
			continue
		}
		if existing, ok := seen[r.Memory.ID]; ok {
			if r.Score > existing.Score {
				existing.Score = r.Score
				existing.Distance = r.Distance
				existing.Memory = r.Memory
			}
			continue
		}
		copyR := *r
		seen[r.Memory.ID] = &copyR
		merged = append(merged, &copyR)
	}

	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})
	return merged
}
