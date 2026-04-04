package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/patterns"
	"github.com/j33pguy/magi/internal/tools"
)

func (s *Server) handleListPatterns(w http.ResponseWriter, r *http.Request) {
	patternList, err := s.analyzePatternsFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	filtered := filterPatterns(patternList, r)
	writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) handleListTrendingPatterns(w http.ResponseWriter, r *http.Request) {
	patternList, err := s.analyzePatternsFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	includeStable := strings.EqualFold(r.URL.Query().Get("include_stable"), "true")
	var filtered []patterns.Pattern
	for _, p := range filterPatterns(patternList, r) {
		if includeStable || p.Trend != string(patterns.TrendStable) {
			filtered = append(filtered, p)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return patternTime(filtered[i].LastSeen).After(patternTime(filtered[j].LastSeen))
	})

	writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) analyzePatternsFromRequest(r *http.Request) ([]patterns.Pattern, error) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 1000
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	var tags []string
	if t := q.Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}

	afterTime, err := tools.ParseTimeParam(q.Get("after"))
	if err != nil {
		return nil, fmt.Errorf("invalid 'after': %v", err)
	}
	beforeTime, err := tools.ParseTimeParam(q.Get("before"))
	if err != nil {
		return nil, fmt.Errorf("invalid 'before': %v", err)
	}

	speaker := q.Get("speaker")
	if speaker == "" {
		speaker = "user"
	}
	if strings.EqualFold(speaker, "all") {
		speaker = ""
	}

	filter := &db.MemoryFilter{
		Project:    q.Get("project"),
		Type:       q.Get("memory_type"),
		Tags:       tags,
		Limit:      limit,
		Offset:     offset,
		Visibility: "", // HTTP API: exclude private by default
		Speaker:    speaker,
		Area:       q.Get("area"),
		SubArea:    q.Get("sub_area"),
		AfterTime:  afterTime,
		BeforeTime: beforeTime,
	}
	applyRequestAccessScope(r, filter)

	memories, err := s.db.ListMemories(filter)
	if err != nil {
		s.logger.Error("listing memories for patterns", "error", err)
		return nil, fmt.Errorf("listing memories: %w", err)
	}

	for _, m := range memories {
		tags, err := s.db.GetTags(m.ID)
		if err != nil {
			continue
		}
		m.Tags = tags
	}

	analyzer := &patterns.Analyzer{}
	return analyzer.Analyze(memories), nil
}

func filterPatterns(items []patterns.Pattern, r *http.Request) []patterns.Pattern {
	q := r.URL.Query()
	wantType := q.Get("type")
	wantTrend := q.Get("trend")
	wantArea := q.Get("pattern_area")
	wantSource := q.Get("source")
	maxCount, _ := strconv.Atoi(q.Get("max_patterns"))

	var filtered []patterns.Pattern
	for _, p := range items {
		if wantType != "" && string(p.Type) != wantType {
			continue
		}
		if wantTrend != "" && p.Trend != wantTrend {
			continue
		}
		if wantArea != "" && p.Area != wantArea {
			continue
		}
		if wantSource != "" && !containsString(p.Sources, wantSource) {
			continue
		}
		filtered = append(filtered, p)
		if maxCount > 0 && len(filtered) >= maxCount {
			break
		}
	}
	return filtered
}

func patternTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.DateTime, value); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return time.Time{}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
