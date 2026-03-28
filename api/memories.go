package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/tools"
)

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	var tags []string
	if t := q.Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}

	afterTime, err := tools.ParseTimeParam(q.Get("after"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid 'after': %v", err)})
		return
	}
	beforeTime, err := tools.ParseTimeParam(q.Get("before"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid 'before': %v", err)})
		return
	}

	filter := &db.MemoryFilter{
		Project:    q.Get("project"),
		Type:       q.Get("type"),
		Tags:       tags,
		Limit:      limit,
		Offset:     offset,
		Visibility: "", // HTTP API: exclude private memories by default
		Speaker:    q.Get("speaker"),
		Area:       q.Get("area"),
		SubArea:    q.Get("sub_area"),
		AfterTime:  afterTime,
		BeforeTime: beforeTime,
	}

	memories, err := s.db.ListMemories(filter)
	if err != nil {
		s.logger.Error("listing memories", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("listing memories: %v", err)})
		return
	}

	// Load tags for each memory
	for _, m := range memories {
		tags, err := s.db.GetTags(m.ID)
		if err != nil {
			s.logger.Error("getting tags", "error", err, "memory_id", m.ID)
			continue
		}
		m.Tags = tags
	}

	writeJSON(w, http.StatusOK, memories)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	// Verify memory exists
	if _, err := s.db.GetMemory(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("memory not found: %v", err)})
		return
	}

	if err := s.db.ArchiveMemory(id); err != nil {
		s.logger.Error("archiving memory", "error", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("archiving memory: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "ok": true})
}
