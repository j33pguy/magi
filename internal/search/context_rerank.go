package search

import (
	"sort"
	"strings"

	"github.com/j33pguy/magi/internal/db"
)

type retrievalContext struct {
	RepositoryCanonical string
}

func applyContextBoosts(store db.Store, filter *db.MemoryFilter, results []*db.HybridResult) {
	reader, ok := store.(db.MemoryContextReader)
	if !ok || len(results) == 0 {
		return
	}

	request := inferRetrievalContext(filter)
	if request == (retrievalContext{}) {
		return
	}

	ids := make([]string, 0, len(results))
	for _, result := range results {
		if result != nil && result.Memory != nil && result.Memory.ID != "" {
			ids = append(ids, result.Memory.ID)
		}
	}
	contexts, err := reader.LookupMemoryContexts(ids)
	if err != nil || len(contexts) == 0 {
		return
	}

	for _, result := range results {
		ctx, ok := contexts[result.Memory.ID]
		if !ok {
			continue
		}
		boost := 0.0
		if request.RepositoryCanonical != "" && request.RepositoryCanonical == ctx.RepositoryCanonical {
			boost += 0.08
		}
		if boost == 0 {
			continue
		}
		result.RRFScore += boost
		if result.Score > 0 {
			result.Score += boost
			if result.Score > 1 {
				result.Score = 1
			}
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].RRFScore == results[j].RRFScore {
			return results[i].Score > results[j].Score
		}
		return results[i].RRFScore > results[j].RRFScore
	})
}

func inferRetrievalContext(filter *db.MemoryFilter) retrievalContext {
	if filter == nil {
		return retrievalContext{}
	}
	ctx := retrievalContext{RepositoryCanonical: parseRepository(filter.Project)}
	for _, tag := range filter.Tags {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(tag, "repo:") {
			if repo := parseRepository(strings.TrimPrefix(tag, "repo:")); repo != "" {
				ctx.RepositoryCanonical = repo
			}
		}
	}
	return ctx
}

func parseRepository(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "repo:")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "ssh://")
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return ""
	}
	if at := strings.LastIndex(raw, "@"); at != -1 {
		raw = raw[at+1:]
	}
	if colon := strings.Index(raw, ":"); colon != -1 {
		left, right := raw[:colon], raw[colon+1:]
		if strings.Contains(right, "/") && !strings.Contains(left, "/") {
			raw = left + "/" + right
		}
	}
	parts := strings.Split(raw, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	if len(parts) == 2 {
		return parts[0] + "/" + parts[1]
	}
	return ""
}
