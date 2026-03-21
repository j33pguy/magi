package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/j33pguy/claude-memory/db"
)

type recallRequest struct {
	Query    string   `json:"query"`
	Project  string   `json:"project"`
	Projects []string `json:"projects"` // multi-namespace: any match
	Type     string   `json:"type"`
	Tags     []string `json:"tags"`
	TopK     int      `json:"top_k"`
}

func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	var req recallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 5
	}

	embedding, err := s.embedder.Embed(r.Context(), req.Query)
	if err != nil {
		s.logger.Error("generating query embedding", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("generating embedding: %v", err)})
		return
	}

	filter := &db.MemoryFilter{
		Project:    req.Project,
		Projects:   req.Projects,
		Type:       req.Type,
		Tags:       req.Tags,
		Visibility: "", // HTTP API: exclude private memories by default
	}

	results, err := s.db.HybridSearch(embedding, req.Query, filter, req.TopK)
	if err != nil {
		s.logger.Error("hybrid search", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("hybrid search: %v", err)})
		return
	}

	// Resolve chunk parents
	for _, result := range results {
		if result.Memory.ParentID != "" {
			parent, err := s.db.GetMemory(result.Memory.ParentID)
			if err == nil {
				result.Memory.Content = parent.Content
				result.Memory.Tags = parent.Tags
			}
		}
	}

	writeJSON(w, http.StatusOK, results)
}
