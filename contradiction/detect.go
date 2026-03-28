// Package contradiction detects potential contradictions between new and existing memories.
package contradiction

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
)

// Candidate is a potentially contradicting memory pair.
type Candidate struct {
	ExistingID      string  `json:"existing_id"`
	ExistingSummary string  `json:"existing_summary"`
	NewContent      string  `json:"new_content"`
	Score           float64 `json:"score"` // contradiction likelihood 0.0–1.0
	Similarity      float64 `json:"similarity"`
	Reason          string  `json:"reason"`
}

// Detector checks new memories against existing ones for potential contradictions.
type Detector struct {
	// Threshold is the minimum similarity above which we flag as potential contradiction.
	// High similarity + same area = possible conflict. Default 0.85.
	Threshold float64
}

// Check runs contradiction detection. It embeds newContent, searches for highly similar
// memories in the same area/sub_area, then applies heuristic filters to identify likely
// contradictions vs just related memories.
func (d *Detector) Check(ctx context.Context, dbClient *db.Client, embedder embeddings.Provider, newContent, area, subArea string) ([]Candidate, error) {
	threshold := d.Threshold
	if threshold <= 0 {
		threshold = 0.85
	}

	// Embed the new content
	embedding, err := embedder.Embed(ctx, newContent)
	if err != nil {
		return nil, fmt.Errorf("embedding new content: %w", err)
	}

	// Search for similar memories in the same area/sub_area
	filter := &db.MemoryFilter{
		Visibility: "all",
	}
	if area != "" {
		filter.Area = area
	}
	if subArea != "" {
		filter.SubArea = subArea
	}

	// Convert similarity threshold to max cosine distance
	maxDistance := 1.0 - threshold

	results, err := dbClient.SearchMemories(embedding, filter, 10)
	if err != nil {
		return nil, fmt.Errorf("searching similar memories: %w", err)
	}

	var candidates []Candidate
	for _, r := range results {
		// Skip results below our similarity threshold
		if r.Distance > maxDistance {
			continue
		}

		similarity := 1.0 - r.Distance
		score, reason := scoreContradiction(newContent, r.Memory.Content)

		if score <= 0.5 {
			continue
		}

		summary := r.Memory.Summary
		if summary == "" {
			summary = truncate(r.Memory.Content, 120)
		}

		candidates = append(candidates, Candidate{
			ExistingID:      r.Memory.ID,
			ExistingSummary: summary,
			NewContent:      truncate(newContent, 120),
			Score:           score,
			Similarity:      similarity,
			Reason:          reason,
		})

		slog.Info("contradiction candidate",
			"existing_id", r.Memory.ID,
			"similarity", fmt.Sprintf("%.2f", similarity),
			"score", fmt.Sprintf("%.2f", score),
			"reason", reason,
		)
	}

	return candidates, nil
}

// scoreContradiction applies heuristics to determine if two similar texts contradict.
// Returns a score (0.0–1.0) and a human-readable reason.
func scoreContradiction(newText, existingText string) (float64, string) {
	newLower := strings.ToLower(newText)
	existingLower := strings.ToLower(existingText)

	var score float64
	var reasons []string

	// Heuristic 1: Numeric value changes near the same keyword
	if s := numericChangeScore(newLower, existingLower); s > 0 {
		score += s
		reasons = append(reasons, "numeric value differs")
	}

	// Heuristic 2: Boolean flips
	if s := booleanFlipScore(newLower, existingLower); s > 0 {
		score += s
		reasons = append(reasons, "boolean/state flip")
	}

	// Heuristic 3: Replacement language in the new text
	if s := replacementScore(newLower); s > 0 {
		score += s
		reasons = append(reasons, "replacement language detected")
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	reason := strings.Join(reasons, "; ")
	if reason == "" {
		reason = "high similarity only"
	}

	return score, reason
}

// numericChangeScore detects when the same keyword appears near different numbers.
// e.g., "VLAN 5" vs "VLAN 150" or "port 8300" vs "port 8301"
func numericChangeScore(newText, existingText string) float64 {
	// Extract keyword-number pairs from both texts
	newPairs := extractKeywordNumbers(newText)
	existingPairs := extractKeywordNumbers(existingText)

	for keyword, newNums := range newPairs {
		if existingNums, ok := existingPairs[keyword]; ok {
			// Same keyword found in both — check if numbers differ
			for _, nn := range newNums {
				for _, en := range existingNums {
					if nn != en {
						return 0.7
					}
				}
			}
		}
	}

	return 0
}

// keywordNumberRe matches patterns like "vlan 5", "port 8300", "version 2.1"
var keywordNumberRe = regexp.MustCompile(`(\b[a-z][a-z_-]+)\s+(\d+(?:\.\d+)?)`)

// extractKeywordNumbers returns a map of keyword → numbers found near it.
func extractKeywordNumbers(text string) map[string][]string {
	pairs := make(map[string][]string)
	matches := keywordNumberRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		keyword := m[1]
		number := m[2]
		pairs[keyword] = append(pairs[keyword], number)
	}
	return pairs
}

// booleanFlipScore detects state reversals like "enabled" vs "disabled".
var booleanPairs = [][2]string{
	{"enabled", "disabled"},
	{"is not", "is"},
	{"true", "false"},
	{"yes", "no"},
	{"active", "inactive"},
	{"on", "off"},
	{"allow", "deny"},
	{"allow", "block"},
	{"open", "closed"},
	{"up", "down"},
}

func booleanFlipScore(newText, existingText string) float64 {
	for _, pair := range booleanPairs {
		a, b := pair[0], pair[1]
		// Check if new has one and existing has the other (in either direction)
		newHasA := containsWord(newText, a)
		newHasB := containsWord(newText, b)
		existHasA := containsWord(existingText, a)
		existHasB := containsWord(existingText, b)

		if (newHasA && existHasB && !existHasA) || (newHasB && existHasA && !existHasB) {
			return 0.6
		}
	}
	return 0
}

// containsWord checks for a whole-word match (bounded by non-alpha chars).
var wordBoundaryCache = make(map[string]*regexp.Regexp)

func containsWord(text, word string) bool {
	re, ok := wordBoundaryCache[word]
	if !ok {
		re = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		wordBoundaryCache[word] = re
	}
	return re.MatchString(text)
}

// replacementScore detects language indicating an update/change.
var replacementPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(changed|moved|migrated)\s+(to|from)\b`),
	regexp.MustCompile(`\bnow\s+(uses?|is|runs?|on|at)\b`),
	regexp.MustCompile(`\bupdated?\s+(to|from)\b`),
	regexp.MustCompile(`\bwas\s+.{1,30},?\s*now\b`),
	regexp.MustCompile(`\bno longer\b`),
	regexp.MustCompile(`\binstead of\b`),
	regexp.MustCompile(`\breplaced?\s+(with|by)\b`),
	regexp.MustCompile(`\bswitched\s+(to|from)\b`),
}

func replacementScore(text string) float64 {
	for _, re := range replacementPatterns {
		if re.MatchString(text) {
			return 0.4
		}
	}
	return 0
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
