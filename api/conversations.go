package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/search"
)

type conversationRequest struct {
	Channel     string   `json:"channel"`
	SessionKey  string   `json:"session_key"`
	StartedAt   string   `json:"started_at"`
	EndedAt     string   `json:"ended_at"`
	TurnCount   int      `json:"turn_count"`
	Topics      []string `json:"topics"`
	Summary     string   `json:"summary"`
	Decisions   []string `json:"decisions"`
	ActionItems []string `json:"action_items"`
}

type conversationSearchRequest struct {
	Query        string  `json:"query"`
	Limit        int     `json:"limit"`
	Channel      string  `json:"channel"`
	MinRelevance float64 `json:"min_relevance"` // 0.0-1.0, filter by score >= threshold
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var req conversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Summary == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "summary is required"})
		return
	}
	if req.Channel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel is required"})
		return
	}

	content := formatConversationContent(&req)

	embedding, err := s.embedder.Embed(r.Context(), content)
	if err != nil {
		s.logger.Error("generating embedding", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("generating embedding: %v", err)})
		return
	}

	memory := &db.Memory{
		Content:    content,
		Summary:    req.Summary,
		Embedding:  embedding,
		Type:       "conversation",
		Visibility: "private",
		Source:     req.Channel,
		TokenCount: len(content) / 4,
	}

	saved, err := s.db.SaveMemory(memory)
	if err != nil {
		s.logger.Error("saving conversation", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("saving conversation: %v", err)})
		return
	}

	tags := []string{"channel:" + req.Channel, "conversation"}
	for _, topic := range req.Topics {
		tags = append(tags, "topic:"+topic)
	}

	resp := map[string]any{"id": saved.ID, "ok": true}
	if err := s.db.SetTags(saved.ID, tags); err != nil {
		// Tags are non-fatal — the conversation memory is saved, tags can be retried.
		s.logger.Warn("setting conversation tags failed (non-fatal)", "error", err, "memory_id", saved.ID)
		tagErr := err.Error()
		resp["tag_warning"] = "tags may not have been saved: " + tagErr[:min(len(tagErr), 80)]
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 10
	}

	tags := []string{"conversation"}
	if channel := q.Get("channel"); channel != "" {
		tags = append(tags, "channel:"+channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Limit:      limit,
		Visibility: "all",
	}

	// Apply "since" filter via listing then filtering — ListMemories returns
	// newest first, so we can stop when we pass the threshold.
	sinceStr := q.Get("since")
	var sinceTime time.Time
	if sinceStr != "" {
		var err error
		sinceTime, err = time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid since timestamp, use RFC3339"})
			return
		}
		// Over-fetch to account for filtering
		filter.Limit = limit * 5
	}

	memories, err := s.db.ListMemories(filter)
	if err != nil {
		s.logger.Error("listing conversations", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("listing conversations: %v", err)})
		return
	}

	for _, m := range memories {
		tags, err := s.db.GetTags(m.ID)
		if err != nil {
			s.logger.Error("getting tags", "error", err, "memory_id", m.ID)
			continue
		}
		m.Tags = tags
	}

	// Filter by since if provided
	if !sinceTime.IsZero() {
		filtered := make([]*db.Memory, 0, len(memories))
		for _, m := range memories {
			created, err := time.Parse(time.DateTime, m.CreatedAt)
			if err != nil {
				continue
			}
			if created.After(sinceTime) || created.Equal(sinceTime) {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
		if len(memories) > limit {
			memories = memories[:limit]
		}
	}

	writeJSON(w, http.StatusOK, memories)
}

func (s *Server) handleSearchConversations(w http.ResponseWriter, r *http.Request) {
	var req conversationSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 5
	}

	var tags []string
	if req.Channel != "" {
		tags = append(tags, "channel:"+req.Channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Visibility: "all",
	}

	resp, err := search.Adaptive(r.Context(), s.db, s.embedder.Embed, req.Query, filter, req.Limit, req.MinRelevance)
	if err != nil {
		s.logger.Error("searching conversations", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("search: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func formatConversationContent(req *conversationRequest) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Conversation on %s", req.Channel))
	if req.SessionKey != "" {
		b.WriteString(fmt.Sprintf(" (session: %s)", req.SessionKey))
	}
	b.WriteString("\n")

	if req.StartedAt != "" || req.EndedAt != "" {
		b.WriteString(fmt.Sprintf("Time: %s to %s\n", req.StartedAt, req.EndedAt))
	}
	if req.TurnCount > 0 {
		b.WriteString(fmt.Sprintf("Turns: %d\n", req.TurnCount))
	}

	if len(req.Topics) > 0 {
		b.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(req.Topics, ", ")))
	}

	b.WriteString("\n")
	b.WriteString(req.Summary)

	if len(req.Decisions) > 0 {
		b.WriteString("\n\nDecisions:\n")
		for _, d := range req.Decisions {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
	}

	if len(req.ActionItems) > 0 {
		b.WriteString("\nAction Items:\n")
		for _, a := range req.ActionItems {
			b.WriteString(fmt.Sprintf("- %s\n", a))
		}
	}

	return b.String()
}
