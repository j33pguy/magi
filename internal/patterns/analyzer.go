// Package patterns detects behavioral patterns from a corpus of memories.
// All analysis is pure heuristics — no LLM calls. Fast and deterministic.
package patterns

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// Analyzer detects behavioral patterns from a corpus of memories.
type Analyzer struct{}

// Analyze takes recent memories (speaker=user) and extracts patterns.
func (a *Analyzer) Analyze(memories []*db.Memory) []Pattern {
	if len(memories) == 0 {
		return nil
	}

	var patterns []Pattern
	patterns = append(patterns, a.detectTechPreferences(memories)...)
	patterns = append(patterns, a.detectDecisionPatterns(memories)...)
	patterns = append(patterns, a.detectWorkTimingPatterns(memories)...)
	patterns = append(patterns, a.detectCommsStyle(memories)...)
	patterns = append(patterns, a.detectTopicBursts(memories)...)
	patterns = append(patterns, a.detectRelationshipPatterns(memories)...)
	patterns = applyTemporalTrends(patterns, memories)
	patterns = applySourceCorrelation(patterns, memories)
	return patterns
}

// --- Technology Preferences ---

type techMention struct {
	name    string
	pattern *regexp.Regexp
}

var techPatterns = []techMention{
	{"Go", regexp.MustCompile(`(?i)\b(in go|use go|written in go|golang|go (binary|module|service|package))\b`)},
	{"gRPC", regexp.MustCompile(`(?i)\bgrpc\b`)},
	{"Terraform", regexp.MustCompile(`(?i)\b(terraform|\.tf\b|hcl)\b`)},
	{"Docker", regexp.MustCompile(`(?i)\b(docker|dockerfile|container)\b`)},
	{"Kubernetes", regexp.MustCompile(`(?i)\b(kubernetes|k8s|kubectl|helm)\b`)},
	{"Compute", regexp.MustCompile(`(?i)\bcompute\b`)},
	{"Vault", regexp.MustCompile(`(?i)\b(hashicorp vault|vault secret|vault token)\b`)},
	{"SQLite", regexp.MustCompile(`(?i)\b(sqlite|libsql|turso)\b`)},
	{"Python", regexp.MustCompile(`(?i)\b(python|\.py\b|pip install)\b`)},
	{"TypeScript", regexp.MustCompile(`(?i)\b(typescript|\.tsx?\b)\b`)},
	{"Rust", regexp.MustCompile(`(?i)\b(rust|cargo|\.rs\b)\b`)},
	{"Nix", regexp.MustCompile(`(?i)\b(nix|nixos|flake\.nix)\b`)},
	{"Ansible", regexp.MustCompile(`(?i)\b(ansible|playbook)\b`)},
}

var preferPattern = regexp.MustCompile(`(?i)(prefer|always use|default to|go[- ]to|switched to|love|fan of)`)
var avoidPattern = regexp.MustCompile(`(?i)(avoid|don'?t use|never use|hate|dislike|moved away from|dropped|ditched)`)

func (a *Analyzer) detectTechPreferences(memories []*db.Memory) []Pattern {
	// Count tech mentions and collect evidence
	type techEvidence struct {
		mentions []string // memory IDs where this tech appears
		positive int      // explicit positive mentions
		negative int      // explicit negative mentions
	}
	techs := make(map[string]*techEvidence)

	for _, m := range memories {
		content := m.Content
		for _, tp := range techPatterns {
			if tp.pattern.MatchString(content) {
				if techs[tp.name] == nil {
					techs[tp.name] = &techEvidence{}
				}
				te := techs[tp.name]
				te.mentions = append(te.mentions, m.ID)

				if preferPattern.MatchString(content) {
					te.positive++
				}
				if avoidPattern.MatchString(content) {
					te.negative++
				}
			}
		}
	}

	var patterns []Pattern
	for name, te := range techs {
		if len(te.mentions) < 3 {
			continue // threshold: appear in 3+ memories
		}

		// Determine if it's a positive preference, negative, or just frequent
		if te.negative > te.positive && te.negative >= 2 {
			patterns = append(patterns, Pattern{
				Type:        PatternPreference,
				Description: fmt.Sprintf("Avoids %s (mentioned negatively in %d memories)", name, te.negative),
				Confidence:  clampConfidence(float64(te.negative) / float64(len(te.mentions))),
				Evidence:    uniqueIDs(te.mentions),
				Area:        "meta",
			})
		} else if te.positive >= 2 {
			patterns = append(patterns, Pattern{
				Type:        PatternPreference,
				Description: fmt.Sprintf("Prefers %s (explicitly favored in %d memories)", name, te.positive),
				Confidence:  clampConfidence(float64(te.positive) / float64(len(te.mentions))),
				Evidence:    uniqueIDs(te.mentions),
				Area:        "meta",
			})
		} else {
			// Frequently mentioned — infer preference from volume
			confidence := clampConfidence(float64(len(te.mentions)) / float64(len(memories)) * 5)
			patterns = append(patterns, Pattern{
				Type:        PatternPreference,
				Description: fmt.Sprintf("Frequently uses %s (mentioned in %d memories)", name, len(te.mentions)),
				Confidence:  confidence,
				Evidence:    uniqueIDs(te.mentions),
				Area:        "meta",
			})
		}
	}
	return patterns
}

// --- Decision Patterns ---

var securityPattern = regexp.MustCompile(`(?i)(security|auth|permission|rbac|token|secret|encrypt|tls|cert|firewall|vpn)`)
var decisionPattern = regexp.MustCompile(`(?i)(decided|going with|settled on|chose|picked|selected|landed on)`)
var comparativePattern = regexp.MustCompile(`(?i)(tradeoff|trade-off|vs\.?|versus|or should|compared|pros and cons|weighing)`)
var questionPattern = regexp.MustCompile(`\?`)

func (a *Analyzer) detectDecisionPatterns(memories []*db.Memory) []Pattern {
	var patterns []Pattern

	securityBeforeDecision := 0
	decisiveCount := 0
	comparativeCount := 0
	var securityEvidence, decisiveEvidence, comparativeEvidence []string

	for _, m := range memories {
		content := m.Content
		hasDecision := decisionPattern.MatchString(content)
		hasSecurity := securityPattern.MatchString(content)
		hasComparative := comparativePattern.MatchString(content)

		if hasSecurity && hasDecision {
			securityBeforeDecision++
			securityEvidence = append(securityEvidence, m.ID)
		}
		if hasDecision {
			decisiveCount++
			decisiveEvidence = append(decisiveEvidence, m.ID)
		}
		if hasComparative {
			comparativeCount++
			comparativeEvidence = append(comparativeEvidence, m.ID)
		}
	}

	if securityBeforeDecision >= 3 {
		patterns = append(patterns, Pattern{
			Type:        PatternDecisionStyle,
			Description: fmt.Sprintf("Considers security implications when making decisions (%d instances)", securityBeforeDecision),
			Confidence:  clampConfidence(float64(securityBeforeDecision) / float64(max(decisiveCount, 1))),
			Evidence:    uniqueIDs(securityEvidence),
			Area:        "meta",
		})
	}

	if comparativeCount >= 3 {
		patterns = append(patterns, Pattern{
			Type:        PatternDecisionStyle,
			Description: fmt.Sprintf("Evaluates tradeoffs and alternatives before deciding (%d comparative discussions)", comparativeCount),
			Confidence:  clampConfidence(float64(comparativeCount) / float64(len(memories)) * 5),
			Evidence:    uniqueIDs(comparativeEvidence),
			Area:        "meta",
		})
	}

	if decisiveCount >= 5 {
		questionCount := 0
		for _, m := range memories {
			if questionPattern.MatchString(m.Content) {
				questionCount++
			}
		}
		ratio := float64(decisiveCount) / float64(max(questionCount, 1))
		if ratio > 2.0 {
			patterns = append(patterns, Pattern{
				Type:        PatternDecisionStyle,
				Description: fmt.Sprintf("Decisive communicator — makes firm decisions (%d decisions vs %d questions)", decisiveCount, questionCount),
				Confidence:  clampConfidence(ratio / 5.0),
				Evidence:    uniqueIDs(decisiveEvidence),
				Area:        "meta",
			})
		}
	}

	return patterns
}

// --- Work Timing Patterns ---

func (a *Analyzer) detectWorkTimingPatterns(memories []*db.Memory) []Pattern {
	var patterns []Pattern

	// Group by day of week
	weekdayArea := make(map[string]int) // area counts on weekdays
	weekendArea := make(map[string]int) // area counts on weekends
	hourCounts := make(map[int]int)     // activity by hour

	for _, m := range memories {
		t, err := parseTime(m.CreatedAt)
		if err != nil {
			continue
		}

		hourCounts[t.Hour()]++

		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			if m.Area != "" {
				weekendArea[m.Area]++
			}
		} else {
			if m.Area != "" {
				weekdayArea[m.Area]++
			}
		}
	}

	// Detect weekend-heavy areas
	for area, weekendCount := range weekendArea {
		weekdayCount := weekdayArea[area]
		// Normalize: 5 weekdays vs 2 weekend days
		weekendRate := float64(weekendCount) / 2.0
		weekdayRate := float64(weekdayCount) / 5.0
		if weekendRate > weekdayRate*1.5 && weekendCount >= 3 {
			var evidence []string
			for _, m := range memories {
				if m.Area == area {
					t, err := parseTime(m.CreatedAt)
					if err == nil && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
						evidence = append(evidence, m.ID)
					}
				}
			}
			patterns = append(patterns, Pattern{
				Type:        PatternWorkPattern,
				Description: fmt.Sprintf("More %s activity on weekends (%.0f%% higher rate)", area, (weekendRate/max(weekdayRate, 0.1)-1)*100),
				Confidence:  clampConfidence(weekendRate / max(weekdayRate, 0.1) / 3.0),
				Evidence:    uniqueIDs(evidence),
				Area:        area,
			})
		}
	}

	// Detect peak hours
	if totalHours := sumValues(hourCounts); totalHours >= 10 {
		peakHour := 0
		peakCount := 0
		for h, c := range hourCounts {
			if c > peakCount {
				peakHour = h
				peakCount = c
			}
		}
		if peakCount >= 5 {
			pct := float64(peakCount) / float64(totalHours) * 100
			var evidence []string
			for _, m := range memories {
				t, err := parseTime(m.CreatedAt)
				if err == nil && t.Hour() == peakHour {
					evidence = append(evidence, m.ID)
				}
			}
			patterns = append(patterns, Pattern{
				Type:        PatternWorkPattern,
				Description: fmt.Sprintf("Peak activity at %02d:00 (%.0f%% of activity)", peakHour, pct),
				Confidence:  clampConfidence(pct / 30.0),
				Evidence:    uniqueIDs(evidence),
				Area:        "meta",
			})
		}
	}

	return patterns
}

// --- Communication Style ---

func (a *Analyzer) detectCommsStyle(memories []*db.Memory) []Pattern {
	if len(memories) < 5 {
		return nil
	}

	var patterns []Pattern
	var totalLen int
	shortCount := 0

	for _, m := range memories {
		contentLen := len(m.Content)
		totalLen += contentLen
		if contentLen < 100 {
			shortCount++
		}
	}

	avgLen := totalLen / len(memories)

	// Concise communicator
	shortPct := float64(shortCount) / float64(len(memories)) * 100
	if shortPct > 50 && avgLen < 200 {
		var evidence []string
		for _, m := range memories {
			if len(m.Content) < 100 {
				evidence = append(evidence, m.ID)
				if len(evidence) >= 10 {
					break
				}
			}
		}
		patterns = append(patterns, Pattern{
			Type:        PatternCommsStyle,
			Description: fmt.Sprintf("Concise communicator — %.0f%% of memories under 100 chars, avg length %d", shortPct, avgLen),
			Confidence:  clampConfidence(shortPct / 80.0),
			Evidence:    evidence,
			Area:        "meta",
		})
	} else if avgLen > 500 {
		var evidence []string
		for _, m := range memories {
			if len(m.Content) > 500 {
				evidence = append(evidence, m.ID)
				if len(evidence) >= 10 {
					break
				}
			}
		}
		patterns = append(patterns, Pattern{
			Type:        PatternCommsStyle,
			Description: fmt.Sprintf("Detailed communicator — average memory length %d chars", avgLen),
			Confidence:  clampConfidence(float64(avgLen) / 1000.0),
			Evidence:    evidence,
			Area:        "meta",
		})
	}

	// Directness: ratio of statements vs questions
	questionCount := 0
	for _, m := range memories {
		if strings.Contains(m.Content, "?") {
			questionCount++
		}
	}
	questionPct := float64(questionCount) / float64(len(memories)) * 100
	if questionPct < 15 && len(memories) >= 10 {
		patterns = append(patterns, Pattern{
			Type:        PatternCommsStyle,
			Description: fmt.Sprintf("Direct communication style — only %.0f%% of memories contain questions", questionPct),
			Confidence:  clampConfidence((100 - questionPct) / 100.0),
			Evidence:    nil,
			Area:        "meta",
		})
	}

	return patterns
}
