package patterns

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PatternType categorizes what kind of behavioral pattern was detected.
type PatternType string

const (
	PatternPreference    PatternType = "preference"
	PatternDecisionStyle PatternType = "decision_style"
	PatternWorkPattern   PatternType = "work_pattern"
	PatternCommsStyle    PatternType = "comms_style"
	PatternTopicBurst    PatternType = "topic_burst"
	PatternRelationship  PatternType = "relationship"
)

// PatternTrend indicates how a pattern changes over time.
type PatternTrend string

const (
	TrendEmerging  PatternTrend = "emerging"
	TrendStable    PatternTrend = "stable"
	TrendDeclining PatternTrend = "declining"
)

// Pattern is a detected behavioral insight.
type Pattern struct {
	Type        PatternType
	Description string
	Confidence  float64  // 0.0–1.0
	Evidence    []string // memory IDs that support this pattern
	Area        string   // which area this relates to
	FirstSeen   string   // timestamp of first evidence (time.DateTime)
	LastSeen    string   // timestamp of most recent evidence (time.DateTime)
	Trend       string   // emerging/stable/declining
	Sources     []string // channels/sources where pattern appears
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{time.DateTime, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}

func clampConfidence(v float64) float64 {
	if v < 0.1 {
		return 0.1
	}
	if v > 0.95 {
		return 0.95
	}
	return v
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func sumValues(m map[int]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, "")
	return s
}

var stopwords = map[string]bool{
	"the": true, "and": true, "with": true, "from": true, "that": true, "this": true,
	"into": true, "over": true, "your": true, "you": true, "for": true, "are": true,
	"was": true, "were": true, "but": true, "not": true, "have": true, "has": true,
	"had": true, "then": true, "than": true, "them": true, "they": true, "their": true,
	"about": true, "using": true, "use": true, "used": true, "also": true, "only": true,
	"just": true, "what": true, "when": true, "where": true, "which": true, "how": true,
	"why": true, "can": true, "could": true, "should": true, "would": true, "will": true,
	"able": true, "may": true, "might": true, "maybe": true, "need": true, "needs": true,
	"want": true, "wants": true, "like": true, "likes": true, "make": true, "made": true,
	"build": true, "built": true, "our": true, "out": true, "in": true, "on": true,
	"of": true, "to": true, "is": true, "it": true, "as": true, "at": true, "by": true,
}
