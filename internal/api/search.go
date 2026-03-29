package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/j33pguy/magi/internal/db"
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("generating embedding: %v", err)})
		return
	}

	filter := &db.MemoryFilter{
		Project:    q.Get("project"),
		Type:       q.Get("type"),
		Tags:       tags,
		Visibility: "", // HTTP API: exclude private memories by default
	}

	results, err := s.db.HybridSearch(embedding, query, filter, topK)
	if err != nil {
		s.logger.Error("hybrid search", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("hybrid search: %v", err)})
		return
	}

	for _, result := range results {
		if result.Memory.ParentID != "" {
			parent, err := s.db.GetMemory(result.Memory.ParentID)
			if err == nil {
				result.Memory.Content = parent.Content
				result.Memory.Tags = parent.Tags
			}
		}
	}

	search.ApplyRecencyWeighting(results, recencyDecay)

	writeJSON(w, http.StatusOK, results)
}
