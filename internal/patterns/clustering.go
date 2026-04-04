package patterns

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

type topicCluster struct {
	label         string
	topics        []string
	evidence      []string
	recentCount   int
	baselineCount int
}

func (a *Analyzer) detectTopicBursts(memories []*db.Memory) []Pattern {
	if len(memories) == 0 {
		return nil
	}

	memoryTopics := make(map[string][]string, len(memories))
	for _, m := range memories {
		topics := extractTopics(m)
		if len(topics) == 0 {
			continue
		}
		memoryTopics[m.ID] = topics
	}
	if len(memoryTopics) == 0 {
		return nil
	}

	// Build co-occurrence counts for clustering.
	cooccur := map[string]map[string]int{}
	for _, topics := range memoryTopics {
		unique := uniqueStrings(topics)
		for i := 0; i < len(unique); i++ {
			for j := i + 1; j < len(unique); j++ {
				a, b := unique[i], unique[j]
				if cooccur[a] == nil {
					cooccur[a] = map[string]int{}
				}
				cooccur[a][b]++
			}
		}
	}

	uf := newUnionFind()
	for t := range memoryTopics {
		_ = t
	}
	for topic, peers := range cooccur {
		for peer, count := range peers {
			if count >= 3 {
				uf.union(topic, peer)
			}
		}
	}

	clusters := map[string]*topicCluster{}
	for _, topics := range memoryTopics {
		for _, topic := range topics {
			root := uf.find(topic)
			cluster := clusters[root]
			if cluster == nil {
				cluster = &topicCluster{}
				clusters[root] = cluster
			}
			cluster.topics = append(cluster.topics, topic)
		}
	}

	// Deduplicate topics and assign labels.
	for _, cluster := range clusters {
		cluster.topics = uniqueStrings(cluster.topics)
		sort.Strings(cluster.topics)
		if len(cluster.topics) > 2 {
			cluster.label = strings.Join(cluster.topics[:2], ", ")
		} else {
			cluster.label = strings.Join(cluster.topics, ", ")
		}
	}

	now := time.Now().UTC()
	recentStart := now.AddDate(0, 0, -7)
	baselineStart := now.AddDate(0, 0, -35)

	for _, m := range memories {
		if m == nil {
			continue
		}
		t, err := parseTime(m.CreatedAt)
		if err != nil {
			continue
		}
		topics, ok := memoryTopics[m.ID]
		if !ok {
			continue
		}
		for _, topic := range topics {
			root := uf.find(topic)
			cluster := clusters[root]
			if cluster == nil {
				continue
			}
			if t.After(recentStart) {
				cluster.recentCount++
				cluster.evidence = append(cluster.evidence, m.ID)
			} else if t.After(baselineStart) {
				cluster.baselineCount++
			}
		}
	}

	var patterns []Pattern
	for _, cluster := range clusters {
		if cluster.recentCount < 5 {
			continue
		}
		baselineAvg := float64(cluster.baselineCount) / 4.0
		if baselineAvg < 1 {
			baselineAvg = 1
		}
		if float64(cluster.recentCount) < baselineAvg*2.0 {
			continue
		}

		description := fmt.Sprintf("Topic burst around %s in last 7 days (%d memories vs avg %.1f/week)", cluster.label, cluster.recentCount, baselineAvg)
		patterns = append(patterns, Pattern{
			Type:        PatternTopicBurst,
			Description: description,
			Confidence:  clampConfidence(float64(cluster.recentCount) / (baselineAvg * 4.0)),
			Evidence:    uniqueIDs(cluster.evidence),
			Area:        dominantArea(memories, cluster.evidence),
		})
	}

	return patterns
}

func extractTopics(m *db.Memory) []string {
	if m == nil {
		return nil
	}

	var topics []string
	if m.Area != "" {
		topics = append(topics, "area:"+normalizeToken(m.Area))
	}
	if m.SubArea != "" {
		topics = append(topics, "subarea:"+normalizeToken(m.SubArea))
	}
	for _, tag := range m.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if strings.HasPrefix(tag, "topic:") {
			val := normalizeToken(strings.TrimPrefix(tag, "topic:"))
			if val != "" {
				topics = append(topics, val)
			}
			continue
		}
		if strings.HasPrefix(tag, "tag:") {
			val := normalizeToken(strings.TrimPrefix(tag, "tag:"))
			if val != "" {
				topics = append(topics, val)
			}
			continue
		}
		if strings.HasPrefix(tag, "channel:") || strings.HasPrefix(tag, "speaker:") || strings.HasPrefix(tag, "pattern") {
			continue
		}
		val := normalizeToken(tag)
		if val != "" {
			topics = append(topics, val)
		}
	}

	if len(topics) == 0 {
		topics = extractKeywords(m.Content)
	}

	return uniqueStrings(topics)
}

func extractKeywords(content string) []string {
	content = strings.ToLower(content)
	parts := strings.FieldsFunc(content, func(r rune) bool {
		return r < 'a' || r > 'z'
	})

	counts := map[string]int{}
	for _, part := range parts {
		if len(part) < 4 {
			continue
		}
		if stopwords[part] {
			continue
		}
		counts[part]++
	}

	type kv struct {
		key   string
		count int
	}
	var all []kv
	for k, v := range counts {
		all = append(all, kv{key: k, count: v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count == all[j].count {
			return all[i].key < all[j].key
		}
		return all[i].count > all[j].count
	})

	limit := 3
	if len(all) < limit {
		limit = len(all)
	}

	keywords := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		keywords = append(keywords, all[i].key)
	}
	return keywords
}

func dominantArea(memories []*db.Memory, evidence []string) string {
	if len(evidence) == 0 {
		return "meta"
	}
	counts := map[string]int{}
	for _, id := range evidence {
		for _, m := range memories {
			if m.ID == id {
				if m.Area != "" {
					counts[m.Area]++
				}
				break
			}
		}
	}
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

type unionFind struct {
	parent map[string]string
}

func newUnionFind() *unionFind {
	return &unionFind{parent: map[string]string{}}
}

func (u *unionFind) find(x string) string {
	if x == "" {
		return ""
	}
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
		return x
	}
	if u.parent[x] == x {
		return x
	}
	u.parent[x] = u.find(u.parent[x])
	return u.parent[x]
}

func (u *unionFind) union(a, b string) {
	ra := u.find(a)
	rb := u.find(b)
	if ra == "" || rb == "" || ra == rb {
		return
	}
	u.parent[rb] = ra
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
