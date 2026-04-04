package patterns

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/j33pguy/magi/internal/db"
)

var entityTokenPattern = regexp.MustCompile(`\b[A-Z][a-z][A-Za-z0-9_\-]{1,}\b`)

func (a *Analyzer) detectRelationshipPatterns(memories []*db.Memory) []Pattern {
	if len(memories) == 0 {
		return nil
	}

	type pairStat struct {
		count    int
		evidence []string
		areas    map[string]int
	}

	pairs := map[string]*pairStat{}
	for _, m := range memories {
		if m == nil {
			continue
		}
		entities := extractEntities(m)
		if len(entities) < 2 {
			continue
		}
		entities = uniqueStrings(entities)
		sort.Strings(entities)
		for i := 0; i < len(entities); i++ {
			for j := i + 1; j < len(entities); j++ {
				key := entities[i] + "::" + entities[j]
				stat := pairs[key]
				if stat == nil {
					stat = &pairStat{areas: map[string]int{}}
					pairs[key] = stat
				}
				stat.count++
				stat.evidence = append(stat.evidence, m.ID)
				if m.Area != "" {
					stat.areas[m.Area]++
				}
			}
		}
	}

	var patterns []Pattern
	for key, stat := range pairs {
		if stat.count < 3 {
			continue
		}
		parts := strings.Split(key, "::")
		if len(parts) != 2 {
			continue
		}
		description := fmt.Sprintf("Frequently co-mentions %s and %s (%d memories)", parts[0], parts[1], stat.count)
		patterns = append(patterns, Pattern{
			Type:        PatternRelationship,
			Description: description,
			Confidence:  clampConfidence(float64(stat.count) / float64(len(memories)) * 4.0),
			Evidence:    uniqueIDs(stat.evidence),
			Area:        dominantAreaFromCounts(stat.areas),
		})
	}

	return patterns
}

func extractEntities(m *db.Memory) []string {
	if m == nil {
		return nil
	}

	entities := map[string]bool{}

	for _, tag := range m.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		for _, prefix := range []string{"tool:", "service:", "person:", "org:", "team:"} {
			if strings.HasPrefix(tag, prefix) {
				val := strings.TrimSpace(strings.TrimPrefix(tag, prefix))
				if val != "" {
					entities[val] = true
				}
			}
		}
	}

	content := m.Content
	for _, match := range entityTokenPattern.FindAllString(content, -1) {
		if skipEntity(match) {
			continue
		}
		entities[match] = true
	}

	for _, tp := range techPatterns {
		if tp.pattern.MatchString(content) {
			entities[tp.name] = true
		}
	}

	var out []string
	for e := range entities {
		out = append(out, e)
	}
	return out
}

func skipEntity(s string) bool {
	lower := strings.ToLower(s)
	if lower == "the" || lower == "this" || lower == "that" || lower == "with" || lower == "from" {
		return true
	}
	if lower == "and" || lower == "for" || lower == "you" || lower == "your" || lower == "they" {
		return true
	}
	if lower == "magi" || lower == "http" || lower == "https" || lower == "json" {
		return true
	}
	return false
}

func dominantAreaFromCounts(counts map[string]int) string {
	best := "meta"
	bestCount := 0
	for area, count := range counts {
		if count > bestCount {
			best = area
			bestCount = count
		}
	}
	return best
}
