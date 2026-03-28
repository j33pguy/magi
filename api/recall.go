package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/search"
	"github.com/j33pguy/claude-memory/tools"
)

type recallRequest struct {
	Query        string   `json:"query"`
	Project      string   `json:"project"`
	Projects     []string `json:"projects"`      // multi-namespace: any match
	Type         string   `json:"type"`
	Tags         []string `json:"tags"`
	TopK         int      `json:"top_k"`
	MinRelevance float64  `json:"min_relevance"`  // 0.0-1.0, filter by score >= threshold
	RecencyDecay float64  `json:"recency_decay"`  // exponential decay rate (0.0 = disabled, 0.01 recommended)
	Speaker      string   `json:"speaker"`
	Area         string   `json:"area"`
	SubArea      string   `json:"sub_area"`
	After        string   `json:"after"`           // only memories after this time (ISO-8601 or relative: 7d, 2w, 1m)
	Before       string   `json:"before"`          // only memories before this time
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

	afterTime, err := tools.ParseTimeParam(req.After)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid 'after': %v", err)})
		return
	}
	beforeTime, err := tools.ParseTimeParam(req.Before)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid 'before': %v", err)})
		return
	}

	filter := &db.MemoryFilter{
		Project:    req.Project,
		Projects:   req.Projects,
		Type:       req.Type,
		Tags:       req.Tags,
		Visibility: "", // HTTP API: exclude private memories by default
		Speaker:    req.Speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		AfterTime:  afterTime,
		BeforeTime: beforeTime,
	}

	resp, err := search.Adaptive(r.Context(), s.db, s.embedder.Embed, req.Query, filter, req.TopK, req.MinRelevance, req.RecencyDecay)
	if err != nil {
		s.logger.Error("adaptive search", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("search: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
