package patterns

import (
	"sort"
	"strings"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func applyTemporalTrends(patterns []Pattern, memories []*db.Memory) []Pattern {
	if len(patterns) == 0 || len(memories) == 0 {
		return patterns
	}

	memoryTimes := make(map[string]time.Time, len(memories))
	for _, m := range memories {
		if m == nil {
			continue
		}
		if t, err := parseTime(m.CreatedAt); err == nil {
			memoryTimes[m.ID] = t
		}
	}

	now := time.Now().UTC()
	recentStart := now.AddDate(0, 0, -30)

	for i := range patterns {
		p := &patterns[i]
		if len(p.Evidence) == 0 {
			continue
		}

		var times []time.Time
		recentCount := 0
		olderCount := 0
		oldest := time.Time{}
		for _, id := range p.Evidence {
			t, ok := memoryTimes[id]
			if !ok {
				continue
			}
			times = append(times, t)
			if oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
			if t.After(recentStart) {
				recentCount++
			} else {
				olderCount++
			}
		}

		if len(times) == 0 {
			continue
		}

		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		p.FirstSeen = times[0].UTC().Format(time.DateTime)
		p.LastSeen = times[len(times)-1].UTC().Format(time.DateTime)

		p.Trend = string(TrendStable)

		if times[0].After(recentStart) && recentCount >= 3 {
			p.Trend = string(TrendEmerging)
			continue
		}

		olderSpanDays := now.Sub(oldest).Hours() / 24
		if olderSpanDays < 7 {
			olderSpanDays = 7
		}
		olderWeeks := olderSpanDays / 7
		if olderWeeks < 1 {
			olderWeeks = 1
		}

		recentWeeks := 30.0 / 7.0
		recentRate := float64(recentCount) / recentWeeks
		olderRate := float64(olderCount) / olderWeeks

		if recentCount >= 3 && olderRate > 0 && recentRate >= olderRate*1.5 {
			p.Trend = string(TrendEmerging)
			continue
		}
		if olderCount >= 3 && recentCount == 0 {
			p.Trend = string(TrendDeclining)
			continue
		}
		if olderCount >= 3 && recentRate > 0 && olderRate >= recentRate*1.3 {
			p.Trend = string(TrendDeclining)
			continue
		}
	}

	return patterns
}

func applySourceCorrelation(patterns []Pattern, memories []*db.Memory) []Pattern {
	if len(patterns) == 0 || len(memories) == 0 {
		return patterns
	}

	memorySources := make(map[string]string, len(memories))
	for _, m := range memories {
		if m == nil {
			continue
		}
		source := strings.TrimSpace(m.Source)
		if source == "" {
			source = "unknown"
		}
		memorySources[m.ID] = source
	}

	for i := range patterns {
		p := &patterns[i]
		if len(p.Evidence) == 0 {
			continue
		}
		seen := map[string]bool{}
		for _, id := range p.Evidence {
			if src, ok := memorySources[id]; ok {
				seen[src] = true
			}
		}
		if len(seen) == 0 {
			continue
		}
		p.Sources = make([]string, 0, len(seen))
		for src := range seen {
			p.Sources = append(p.Sources, src)
		}
		sort.Strings(p.Sources)
		if len(p.Sources) > 1 {
			boost := 1.0 + 0.1*float64(len(p.Sources)-1)
			p.Confidence = clampConfidence(p.Confidence * boost)
		}
	}

	return patterns
}
