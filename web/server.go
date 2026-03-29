// Package web provides an embedded web GUI for browsing claude-memory.
package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j33pguy/claude-memory/classify"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
	"github.com/j33pguy/claude-memory/patterns"
"github.com/j33pguy/claude-memory/ingest"
)

const pageSize = 30

// RegisterRoutes registers all web UI routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, dbClient *db.Client, embedder embeddings.Provider, logger *slog.Logger) {
	tmpl, pages := parseTemplates()
	h := &handler{db: dbClient, embedder: embedder, logger: logger, tmpl: tmpl, pages: pages}

	// Pages
	mux.HandleFunc("GET /", h.listPage)
	mux.HandleFunc("GET /search", h.searchPage)
	mux.HandleFunc("GET /memory/{id}", h.detailPage)
	mux.HandleFunc("GET /memory/{id}/partial", h.memoryPartial)
	mux.HandleFunc("GET /new", h.newPage)
	mux.HandleFunc("GET /stats", h.statsPage)
	mux.HandleFunc("GET /graph", h.graphPage)

	mux.HandleFunc("GET /patterns", h.patternsPage)
// Ingest
	mux.HandleFunc("GET /ingest", h.ingestPage)
	mux.HandleFunc("POST /ingest", h.handleIngest)
	mux.HandleFunc("POST /api/ingest/detect", h.handleDetectFormat)

	// API endpoints
	mux.HandleFunc("GET /api/memories", h.apiMemories)
	mux.HandleFunc("GET /api/search", h.apiSearch)
	mux.HandleFunc("GET /api/stats", h.apiStats)
	mux.HandleFunc("POST /api/memories", h.apiCreateMemory)
	mux.HandleFunc("DELETE /api/memories/{id}", h.apiDeleteMemory)
	mux.HandleFunc("GET /api/memories/{id}/related", h.apiRelatedMemories)
	mux.HandleFunc("GET /api/graph", h.apiGraph)

	// Conversations
	mux.HandleFunc("GET /conversations", h.conversationsPage)
	mux.HandleFunc("GET /api/conversations/list", h.apiConversationsList)
	mux.HandleFunc("POST /api/conversations/search", h.apiConversationsSearch)
mux.HandleFunc("POST /api/analyze-patterns", h.apiAnalyzePatterns)
}

type handler struct {
	db       *db.Client
	embedder embeddings.Provider
	logger   *slog.Logger
	tmpl     *template.Template
	pages    map[string]*template.Template
}

// --- Template helpers ---

func parseTemplates() (*template.Template, map[string]*template.Template) {
	funcMap := template.FuncMap{
		"truncate":     truncate,
		"formatDate":   formatDate,
		"speakerBadge": speakerBadge,
		"areaBadge":    areaBadge,
		"speakerColor": speakerColor,
		"areaColor":    areaColor,
		"channelBadge": channelBadge,
		"isTopicTag":   isTopicTag,
		"stripPrefix":  strings.TrimPrefix,
	}

	// Parse base template
	base := template.Must(
		template.New("").Funcs(funcMap).ParseFS(WebFS, "templates/base.html"),
	)

	// Create per-page clones so {{define "content"}} blocks don't collide
	pageFiles := []string{
		"templates/list.html",
		"templates/search.html",
		"templates/detail.html",
		"templates/new.html",
		"templates/stats.html",
		"templates/graph.html",
		"templates/conversations.html",
		"templates/patterns.html",
		"templates/ingest.html",
	}

	pages := make(map[string]*template.Template, len(pageFiles))
	for _, pf := range pageFiles {
		clone := template.Must(base.Clone())
		template.Must(clone.ParseFS(WebFS, pf))
		// Key is the page name without path/extension
		name := strings.TrimPrefix(pf, "templates/")
		name = strings.TrimSuffix(name, ".html")
		pages[name] = clone
	}

	// Also keep a combined set for partial templates (conv_rows, search_results, etc.)
	combined := template.Must(
		template.New("").Funcs(funcMap).ParseFS(WebFS, "templates/*.html"),
	)

	return combined, pages
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func formatDate(s string) string {
	for _, layout := range []string{time.DateTime, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			now := time.Now()
			diff := now.Sub(t)
			switch {
			case diff < time.Minute:
				return "just now"
			case diff < time.Hour:
				return fmt.Sprintf("%dm ago", int(diff.Minutes()))
			case diff < 24*time.Hour:
				return fmt.Sprintf("%dh ago", int(diff.Hours()))
			case diff < 7*24*time.Hour:
				return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
			default:
				return t.Format("Jan 2, 2006")
			}
		}
	}
	return s
}

func speakerBadge(s string) string {
	switch s {
	case "j33p":
		return "badge-green"
	case "gilfoyle":
		return "badge-purple"
	case "agent":
		return "badge-blue"
	case "system":
		return "badge-gray"
	default:
		return "badge-gray"
	}
}

func areaBadge(s string) string {
	switch s {
	case "work":
		return "badge-blue"
	case "homelab":
		return "badge-amber"
	case "home":
		return "badge-green"
	case "family":
		return "badge-pink"
	case "project":
		return "badge-purple"
	case "meta":
		return "badge-gray"
	default:
		return "badge-gray"
	}
}

func speakerColor(s string) string {
	switch s {
	case "j33p":
		return "#10b981"
	case "gilfoyle":
		return "#a855f7"
	case "agent":
		return "#3b82f6"
	case "system":
		return "#64748b"
	default:
		return "#64748b"
	}
}

func areaColor(s string) string {
	switch s {
	case "work":
		return "#3b82f6"
	case "homelab":
		return "#f59e0b"
	case "home":
		return "#10b981"
	case "family":
		return "#ec4899"
	case "project":
		return "#a855f7"
	case "meta":
		return "#64748b"
	default:
		return "#64748b"
	}
}

// --- Page handlers ---

type listData struct {
	Nav        string
	Memories   []*db.Memory
	Filter     db.MemoryFilter
	HasMore    bool
	NextOffset int
}

func (h *handler) listPage(w http.ResponseWriter, r *http.Request) {
	filter := filterFromQuery(r)
	filter.Limit = pageSize + 1
	filter.Visibility = "all"

	memories, err := h.db.ListMemories(&filter)
	if err != nil {
		h.serverError(w, err)
		return
	}

	hasMore := len(memories) > pageSize
	if hasMore {
		memories = memories[:pageSize]
	}

	data := listData{
		Nav:        "list",
		Memories:   memories,
		Filter:     filter,
		HasMore:    hasMore,
		NextOffset: filter.Offset + pageSize,
	}
	h.render(w, "base", data)
}

func (h *handler) searchPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "base", map[string]string{"Nav": "search"})
}

func (h *handler) detailPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mem, err := h.db.GetMemory(id)
	if err != nil {
		h.serverError(w, err)
		return
	}

	data := struct {
		Nav    string
		Memory *db.Memory
	}{Nav: "detail", Memory: mem}
	h.render(w, "base", data)
}

func (h *handler) newPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "base", map[string]string{"Nav": "new"})
}

func (h *handler) statsPage(w http.ResponseWriter, r *http.Request) {
	stats, err := h.getStats()
	if err != nil {
		h.serverError(w, err)
		return
	}
	stats.Nav = "stats"
	h.render(w, "base", stats)
}

func (h *handler) graphPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "base", map[string]string{"Nav": "graph"})
}

// --- HTMX partial handlers ---

func (h *handler) memoryPartial(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mem, err := h.db.GetMemory(id)
	if err != nil {
		h.serverError(w, err)
		return
	}

	data := struct{ Memory *db.Memory }{Memory: mem}
	h.renderPartial(w, "memory_expanded", data)
}

// --- API handlers ---

func (h *handler) apiMemories(w http.ResponseWriter, r *http.Request) {
	filter := filterFromQuery(r)
	filter.Limit = pageSize + 1
	filter.Visibility = "all"

	memories, err := h.db.ListMemories(&filter)
	if err != nil {
		h.serverError(w, err)
		return
	}

	hasMore := len(memories) > pageSize
	if hasMore {
		memories = memories[:pageSize]
	}

	// If Accept: application/json, return JSON
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(memories)
		return
	}

	data := listData{
		Memories:   memories,
		Filter:     filter,
		HasMore:    hasMore,
		NextOffset: filter.Offset + pageSize,
	}
	h.renderPartial(w, "memory_rows", data)
}

type searchResult struct {
	Memory       *db.Memory
	ScorePercent float64
}

func (h *handler) apiSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		h.renderPartial(w, "search_results", struct {
			Results []searchResult
			Query   string
		}{})
		return
	}

	embedding, err := h.embedder.Embed(r.Context(), q)
	if err != nil {
		h.serverError(w, err)
		return
	}

	filter := &db.MemoryFilter{Visibility: "all"}
	results, err := h.db.HybridSearch(embedding, q, filter, 20)
	if err != nil {
		h.serverError(w, err)
		return
	}

	var sResults []searchResult
	for _, r := range results {
		score := r.Score * 100
		if score < 0 {
			score = 0
		}
		sResults = append(sResults, searchResult{
			Memory:       r.Memory,
			ScorePercent: score,
		})
	}

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sResults)
		return
	}

	data := struct {
		Results []searchResult
		Query   string
	}{Results: sResults, Query: q}
	h.renderPartial(w, "search_results", data)
}

func (h *handler) apiCreateMemory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	mem := &db.Memory{
		Content: content,
		Summary: strings.TrimSpace(r.FormValue("summary")),
		Type:    r.FormValue("type"),
		Speaker: r.FormValue("speaker"),
		Area:    r.FormValue("area"),
		SubArea: r.FormValue("sub_area"),
		Project: r.FormValue("project"),
	}
	if mem.Type == "" {
		mem.Type = "note"
	}

	// Generate embedding
	embedding, err := h.embedder.Embed(r.Context(), content)
	if err != nil {
		h.serverError(w, err)
		return
	}
	mem.Embedding = embedding
	mem.TokenCount = len(strings.Fields(content))

	saved, err := h.db.SaveMemory(mem)
	if err != nil {
		h.serverError(w, err)
		return
	}

	// Handle tags
	tagsStr := strings.TrimSpace(r.FormValue("tags"))
	if tagsStr != "" {
		var tags []string
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		if len(tags) > 0 {
			if err := h.db.SetTags(saved.ID, tags); err != nil {
				h.logger.Error("failed to set tags", "error", err)
			}
		}
	}

	h.renderPartial(w, "create_success", saved)
}

func (h *handler) apiDeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.DeleteMemory(id); err != nil {
		h.serverError(w, err)
		return
	}
	// Return empty so HTMX removes the element
	w.WriteHeader(http.StatusOK)
}

type relatedMemoryResult struct {
	Memory *db.Memory      `json:"memory"`
	Links  []*db.MemoryLink `json:"links"`
}

func (h *handler) apiRelatedMemories(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	links, err := h.db.GetLinks(r.Context(), id, "both")
	if err != nil {
		h.serverError(w, err)
		return
	}

	// Collect unique neighbor IDs
	seen := map[string]bool{}
	var neighborIDs []string
	for _, l := range links {
		neighborID := l.ToID
		if neighborID == id {
			neighborID = l.FromID
		}
		if !seen[neighborID] {
			seen[neighborID] = true
			neighborIDs = append(neighborIDs, neighborID)
		}
	}

	var results []relatedMemoryResult
	for _, nid := range neighborIDs {
		mem, err := h.db.GetMemory(nid)
		if err != nil {
			continue
		}
		var relevantLinks []*db.MemoryLink
		for _, l := range links {
			if l.FromID == nid || l.ToID == nid {
				relevantLinks = append(relevantLinks, l)
			}
		}
		results = append(results, relatedMemoryResult{Memory: mem, Links: relevantLinks})
	}

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
		return
	}

	h.renderPartial(w, "related_memories", results)
}

func (h *handler) apiGraph(w http.ResponseWriter, r *http.Request) {
	memories, links, err := h.db.GetGraphData(r.Context(), 300)
	if err != nil {
		h.serverError(w, err)
		return
	}

	type graphNode struct {
		ID        string `json:"id"`
		Summary   string `json:"summary"`
		Area      string `json:"area"`
		Speaker   string `json:"speaker"`
		Type      string `json:"type"`
		CreatedAt string `json:"created_at"`
		LinkCount int    `json:"link_count"`
	}
	type graphEdge struct {
		ID       string  `json:"id"`
		From     string  `json:"from"`
		To       string  `json:"to"`
		Relation string  `json:"relation"`
		Weight   float64 `json:"weight"`
		Auto     bool    `json:"auto"`
	}

	nodes := make([]graphNode, 0, len(memories))
	for _, m := range memories {
		nodes = append(nodes, graphNode{
			ID:        m.ID,
			Summary:   m.Summary,
			Area:      m.Area,
			Speaker:   m.Speaker,
			Type:      m.Type,
			CreatedAt: m.CreatedAt,
		})
	}

	edges := make([]graphEdge, 0, len(links))
	for _, l := range links {
		edges = append(edges, graphEdge{
			ID:       l.ID,
			From:     l.FromID,
			To:       l.ToID,
			Relation: l.Relation,
			Weight:   l.Weight,
			Auto:     l.Auto,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"nodes": nodes,
		"edges": edges,
	})
}

type statsData struct {
	Nav           string
	TotalMemories int
	ThisWeek      int
	TopArea       string
	SpeakerCounts []countRow
	AreaCounts    []countRow
	TopTags       []countRow
}

type countRow struct {
	Name    string
	Count   int
	Percent float64
}

func (h *handler) apiStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.getStats()
	if err != nil {
		h.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *handler) getStats() (*statsData, error) {
	stats := &statsData{}

	// Total memories
	err := h.db.DB.QueryRow("SELECT COUNT(*) FROM memories WHERE archived_at IS NULL").Scan(&stats.TotalMemories)
	if err != nil {
		return nil, fmt.Errorf("counting memories: %w", err)
	}

	// This week
	weekAgo := time.Now().AddDate(0, 0, -7).UTC().Format(time.DateTime)
	err = h.db.DB.QueryRow("SELECT COUNT(*) FROM memories WHERE archived_at IS NULL AND created_at >= ?", weekAgo).Scan(&stats.ThisWeek)
	if err != nil {
		return nil, fmt.Errorf("counting this week: %w", err)
	}

	// Speaker breakdown
	rows, err := h.db.DB.Query("SELECT speaker, COUNT(*) as cnt FROM memories WHERE archived_at IS NULL GROUP BY speaker ORDER BY cnt DESC")
	if err != nil {
		return nil, fmt.Errorf("speaker counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c countRow
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		stats.SpeakerCounts = append(stats.SpeakerCounts, c)
	}
	rows.Close()

	// Area breakdown
	rows, err = h.db.DB.Query("SELECT area, COUNT(*) as cnt FROM memories WHERE archived_at IS NULL GROUP BY area ORDER BY cnt DESC")
	if err != nil {
		return nil, fmt.Errorf("area counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c countRow
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		stats.AreaCounts = append(stats.AreaCounts, c)
	}
	rows.Close()

	// Calculate percentages
	if stats.TotalMemories > 0 {
		for i := range stats.SpeakerCounts {
			stats.SpeakerCounts[i].Percent = float64(stats.SpeakerCounts[i].Count) / float64(stats.TotalMemories) * 100
		}
		for i := range stats.AreaCounts {
			stats.AreaCounts[i].Percent = float64(stats.AreaCounts[i].Count) / float64(stats.TotalMemories) * 100
		}
	}

	// Top area
	if len(stats.AreaCounts) > 0 {
		// Skip empty area
		for _, a := range stats.AreaCounts {
			if a.Name != "" {
				stats.TopArea = a.Name
				break
			}
		}
	}

	// Top 10 tags
	rows, err = h.db.DB.Query("SELECT tag, COUNT(*) as cnt FROM memory_tags GROUP BY tag ORDER BY cnt DESC LIMIT 10")
	if err != nil {
		return nil, fmt.Errorf("tag counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c countRow
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		stats.TopTags = append(stats.TopTags, c)
	}

	return stats, nil
}

// --- Conversation handlers ---

type dateGroup struct {
	Label         string
	Conversations []*db.Memory
}

type conversationsData struct {
	Nav           string
	Conversations []*db.Memory
	DateGroups    []dateGroup
}

func (h *handler) conversationsPage(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")

	tags := []string{"conversation"}
	if channel != "" {
		tags = append(tags, "channel:"+channel)
	}

	memories, err := h.db.ListMemories(&db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Limit:      50,
		Visibility: "all",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := conversationsData{
		Nav:           "conversations",
		Conversations: memories,
		DateGroups:    groupByDate(memories),
	}
	h.render(w, "base", data)
}

// --- Patterns ---

type patternGroup struct {
	Type     string
	Label    string
	Patterns []*db.Memory
}

type patternsData struct {
	Nav    string
	Groups []patternGroup
}

func (h *handler) patternsPage(w http.ResponseWriter, r *http.Request) {
	memories, err := h.db.ListMemories(&db.MemoryFilter{
		Tags:       []string{"pattern"},
		Limit:      200,
		Visibility: "all",
	})
	if err != nil {
		h.serverError(w, err)
		return
	}

	for _, m := range memories {
		t, err := h.db.GetTags(m.ID)
		if err != nil {
			continue
		}
		m.Tags = t
	}

	grouped := map[string]*patternGroup{
		"preference":     {Type: "preference", Label: "Technology Preferences"},
		"decision_style": {Type: "decision_style", Label: "Decision Style"},
		"work_pattern":   {Type: "work_pattern", Label: "Work Patterns"},
		"comms_style":    {Type: "comms_style", Label: "Communication Style"},
	}

	for _, m := range memories {
		for _, tag := range m.Tags {
			if strings.HasPrefix(tag, "pattern_type:") {
				pType := strings.TrimPrefix(tag, "pattern_type:")
				if g, ok := grouped[pType]; ok {
					g.Patterns = append(g.Patterns, m)
				}
			}
		}
	}

	var groups []patternGroup
	for _, key := range []string{"preference", "decision_style", "work_pattern", "comms_style"} {
		if g := grouped[key]; len(g.Patterns) > 0 {
			groups = append(groups, *g)
		}
	}

	h.render(w, "base", patternsData{Nav: "patterns", Groups: groups})
}

func (h *handler) apiConversationsList(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")

	tags := []string{"conversation"}
	if channel != "" {
		tags = append(tags, "channel:"+channel)
	}

	memories, err := h.db.ListMemories(&db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Limit:      50,
		Visibility: "all",
	})
	if err != nil {
		h.serverError(w, err)
		return
	}

	for _, m := range memories {
		t, err := h.db.GetTags(m.ID)
		if err != nil {
			continue
		}
		m.Tags = t
	}

	data := conversationsData{
		Conversations: memories,
		DateGroups:    groupByDate(memories),
	}
	h.renderPartial(w, "conv_rows", data)
}

func (h *handler) apiConversationsSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query   string `json:"query"`
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.renderPartial(w, "conv_search_results", struct{ Results []searchResult }{})
		return
	}

	if req.Query == "" {
		// Empty query — return normal list
		h.apiConversationsList(w, r)
		return
	}

	embedding, err := h.embedder.Embed(r.Context(), req.Query)
	if err != nil {
		h.serverError(w, err)
		return
	}

	var filterTags []string
	if req.Channel != "" {
		filterTags = append(filterTags, "channel:"+req.Channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       filterTags,
		Visibility: "all",
	}
	results, err := h.db.HybridSearch(embedding, req.Query, filter, 20)
	if err != nil {
		h.serverError(w, err)
		return
	}

	var sResults []searchResult
	for _, r := range results {
		if r.Memory.Tags == nil {
			t, err := h.db.GetTags(r.Memory.ID)
			if err == nil {
				r.Memory.Tags = t
			}
		}
		score := r.Score * 100
		if score < 0 {
			score = 0
		}
		sResults = append(sResults, searchResult{
			Memory:       r.Memory,
			ScorePercent: score,
		})
	}

	h.renderPartial(w, "conv_search_results", struct{ Results []searchResult }{Results: sResults})
}

func groupByDate(memories []*db.Memory) []dateGroup {
	groups := make(map[string]*dateGroup)
	var order []string

	for _, m := range memories {
		var dayLabel string
		parsed := false
		for _, layout := range []string{time.DateTime, time.RFC3339} {
			if t, err := time.Parse(layout, m.CreatedAt); err == nil {
				now := time.Now()
				today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
				memDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

				switch {
				case memDay.Equal(today):
					dayLabel = "Today"
				case memDay.Equal(today.AddDate(0, 0, -1)):
					dayLabel = "Yesterday"
				case memDay.After(today.AddDate(0, 0, -7)):
					dayLabel = t.Format("Monday")
				default:
					dayLabel = t.Format("January 2, 2006")
				}
				parsed = true
				break
			}
		}
		if !parsed {
			dayLabel = "Unknown"
		}

		if _, ok := groups[dayLabel]; !ok {
			groups[dayLabel] = &dateGroup{Label: dayLabel}
			order = append(order, dayLabel)
		}
		groups[dayLabel].Conversations = append(groups[dayLabel].Conversations, m)
	}

	result := make([]dateGroup, 0, len(order))
	for _, key := range order {
		result = append(result, *groups[key])
	}
	return result
}

func channelBadge(s string) string {
	switch s {
	case "discord":
		return "badge-discord"
	case "webchat":
		return "badge-webchat"
	case "claude-code":
		return "badge-claude-code"
	case "slack":
		return "badge-slack"
	default:
		return "badge-channel"
	}
}

func isTopicTag(s string) bool {
	return strings.HasPrefix(s, "topic:")
}

// --- Ingest handlers ---

func (h *handler) ingestPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "base", map[string]string{"Nav": "ingest"})
}

type ingestResponse struct {
	Imported int              `json:"imported"`
	Skipped  int              `json:"skipped"`
	Memories []ingestMemoryRef `json:"memories,omitempty"`
	Error    string           `json:"error,omitempty"`
}

type ingestMemoryRef struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

func (h *handler) handleIngest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	body, err := io.ReadAll(io.LimitReader(r.Body, ingest.MaxInputSize+1))
	if err != nil {
		json.NewEncoder(w).Encode(ingestResponse{Error: "failed to read body"})
		return
	}
	if len(body) > ingest.MaxInputSize {
		json.NewEncoder(w).Encode(ingestResponse{Error: "input too large (max 10MB)"})
		return
	}

	conv, err := ingest.Parse(body)
	if err != nil {
		json.NewEncoder(w).Encode(ingestResponse{Error: fmt.Sprintf("parse error: %v", err)})
		return
	}

	candidates := ingest.ExtractMemories(conv)

	dedup := &ingest.Deduplicator{DB: h.db, Embedder: h.embedder}
	kept, skipped, err := dedup.Filter(r.Context(), candidates)
	if err != nil {
		json.NewEncoder(w).Encode(ingestResponse{Error: fmt.Sprintf("dedup error: %v", err)})
		return
	}

	resp := ingestResponse{Skipped: skipped}
	for _, em := range kept {
		c := classify.Infer(em.Content)
		embedding, err := h.embedder.Embed(r.Context(), em.Content)
		if err != nil {
			h.logger.Error("embedding failed during ingest", "error", err)
			continue
		}

		mem := &db.Memory{
			Content:    em.Content,
			Summary:    em.Summary,
			Embedding:  embedding,
			Type:       em.Type,
			Source:     em.Source,
			Speaker:    em.Speaker,
			Area:       c.Area,
			SubArea:    c.SubArea,
			TokenCount: len(em.Content) / 4,
		}

		saved, err := h.db.SaveMemory(mem)
		if err != nil {
			h.logger.Error("save failed during ingest", "error", err)
			continue
		}

		tags := append(em.Tags, "speaker:"+em.Speaker)
		if c.Area != "" {
			tags = append(tags, "area:"+c.Area)
		}
		if c.SubArea != "" {
			tags = append(tags, "sub_area:"+c.SubArea)
		}
		if err := h.db.SetTags(saved.ID, tags); err != nil {
			h.logger.Error("set tags failed during ingest", "error", err)
		}

		resp.Imported++
		resp.Memories = append(resp.Memories, ingestMemoryRef{
			ID:      saved.ID,
			Type:    em.Type,
			Summary: em.Summary,
		})
	}

	json.NewEncoder(w).Encode(resp)
}

type detectResponse struct {
	Format string `json:"format"`
	Turns  int    `json:"turns,omitempty"`
}

func (h *handler) handleDetectFormat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	body, err := io.ReadAll(io.LimitReader(r.Body, ingest.MaxInputSize+1))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	format := ingest.Detect(body)
	resp := detectResponse{Format: string(format)}

	// Try to parse to get turn count
	if conv, err := ingest.Parse(body); err == nil {
		resp.Turns = len(conv.Turns)
	}

	json.NewEncoder(w).Encode(resp)
}


func (h *handler) apiAnalyzePatterns(w http.ResponseWriter, r *http.Request) {
	// Fetch last 90 days of j33p memories
	since := time.Now().AddDate(0, 0, -90)
	memories, err := h.db.ListMemories(&db.MemoryFilter{
		Speaker:    "j33p",
		AfterTime:  &since,
		Limit:      1000,
		Visibility: "all",
	})
	if err != nil {
		h.serverError(w, err)
		return
	}

	// Run analyzer
	analyzer := &patterns.Analyzer{}
	detected := analyzer.Analyze(memories)

	// Store patterns
	stored, skippedDups, err := patterns.StorePatterns(r.Context(), h.db, h.embedder, detected)
	if err != nil {
		h.serverError(w, err)
		return
	}

	result := map[string]int{
		"patterns_found":     len(detected),
		"patterns_stored":    len(stored),
		"skipped_duplicates": skippedDups,
	}

	// If HTMX request, re-render the patterns page content
	if r.Header.Get("HX-Request") == "true" {
		// Redirect to patterns page via HTMX
		w.Header().Set("HX-Redirect", "/patterns")
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// --- Helpers ---

func filterFromQuery(r *http.Request) db.MemoryFilter {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	return db.MemoryFilter{
		Speaker: q.Get("speaker"),
		Area:    q.Get("area"),
		SubArea: q.Get("sub_area"),
		Type:    q.Get("type"),
		Offset:  offset,
	}
}

func (h *handler) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if name == "base" {
		// Determine page from Nav field
		page := ""
		switch v := data.(type) {
		case map[string]interface{}:
			if n, ok := v["Nav"].(string); ok {
				page = n
			}
		case map[string]string:
			page = v["Nav"]
		default:
			// Use reflection-free approach: check known types
			page = getNavFromData(data)
		}
		if t, ok := h.pages[page]; ok {
			if err := t.ExecuteTemplate(w, "base", data); err != nil {
				h.logger.Error("template render error", "page", page, "error", err)
			}
			return
		}
	}
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.logger.Error("template render error", "template", name, "error", err)
	}
}

// getNavFromData extracts the Nav field from known data structs.
func getNavFromData(data any) string {
	switch v := data.(type) {
	case conversationsData:
		return v.Nav
	case *conversationsData:
		return v.Nav
	case patternsData:
		return v.Nav
	case *patternsData:
		return v.Nav
	case statsData:
		return v.Nav
	case *statsData:
		return v.Nav
	case listData:
		return v.Nav
	case *listData:
		return v.Nav
	}
	// Fallback: try to get Nav via fmt
	type hasNav interface{ GetNav() string }
	if n, ok := data.(hasNav); ok {
		return n.GetNav()
	}
	return ""
}

func (h *handler) renderPartial(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.logger.Error("partial render error", "template", name, "error", err)
	}
}

func (h *handler) serverError(w http.ResponseWriter, err error) {
	h.logger.Error("server error", "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
