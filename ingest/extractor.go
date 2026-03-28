package ingest

import (
	"regexp"
	"strings"
)

// ExtractedMemory is a memory extracted from a conversation, ready for storage.
type ExtractedMemory struct {
	Content string
	Type    string   // "decision" | "lesson" | "preference" | "project_context" | "conversation"
	Summary string
	Speaker string
	Tags    []string
	Source  string // "grok", "chatgpt", etc.
}

// ExtractMemories analyzes a ParsedConversation and extracts meaningful memories.
func ExtractMemories(conv *ParsedConversation) []ExtractedMemory {
	if conv == nil || len(conv.Turns) == 0 {
		return nil
	}

	var memories []ExtractedMemory

	for i, turn := range conv.Turns {
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}

		speaker := mapSpeaker(turn.Role, conv.Source)

		// Check heuristic categories in priority order
		if matchesDecision(content) {
			memories = append(memories, ExtractedMemory{
				Content: content,
				Type:    "decision",
				Summary: summarize(content),
				Speaker: speaker,
				Tags:    buildTags(conv.Source, "decision"),
				Source:  conv.Source,
			})
			continue
		}

		if matchesLesson(content) {
			memories = append(memories, ExtractedMemory{
				Content: content,
				Type:    "lesson",
				Summary: summarize(content),
				Speaker: speaker,
				Tags:    buildTags(conv.Source, "lesson"),
				Source:  conv.Source,
			})
			continue
		}

		if matchesPreference(content) {
			memories = append(memories, ExtractedMemory{
				Content: content,
				Type:    "preference",
				Summary: summarize(content),
				Speaker: speaker,
				Tags:    buildTags(conv.Source, "preference"),
				Source:  conv.Source,
			})
			continue
		}

		// Project context: first 3 turns of conversation
		if i < 3 {
			memories = append(memories, ExtractedMemory{
				Content: content,
				Type:    "project_context",
				Summary: summarize(content),
				Speaker: speaker,
				Tags:    buildTags(conv.Source, "project_context"),
				Source:  conv.Source,
			})
			continue
		}

		// General conversation: only if > 200 chars
		if len(content) > 200 {
			memories = append(memories, ExtractedMemory{
				Content: content,
				Type:    "conversation",
				Summary: summarize(content),
				Speaker: speaker,
				Tags:    buildTags(conv.Source, "conversation"),
				Source:  conv.Source,
			})
		}
	}

	return memories
}

// --- Heuristic matchers ---

var decisionPatterns = regexp.MustCompile(`(?i)\b(decided|will use|going with|switching to|chose|settled on)\b`)

func matchesDecision(content string) bool {
	return decisionPatterns.MatchString(content)
}

var lessonPatterns = regexp.MustCompile(`(?i)\b(learned|figured out|turns out|the issue was|fixed by|root cause)\b`)

func matchesLesson(content string) bool {
	return lessonPatterns.MatchString(content)
}

var preferencePatterns = regexp.MustCompile(`(?i)\b(I prefer|I like|I want|always use|never use|don't use|do not use)\b`)

func matchesPreference(content string) bool {
	return preferencePatterns.MatchString(content)
}

// --- Helpers ---

func summarize(content string) string {
	// First 100 chars, collapse whitespace
	s := strings.Join(strings.Fields(content), " ")
	if len(s) > 100 {
		return s[:100]
	}
	return s
}

func mapSpeaker(role, source string) string {
	if role == "user" {
		return "j33p"
	}
	switch source {
	case "grok":
		return "grok"
	case "chatgpt":
		return "chatgpt"
	default:
		return "gilfoyle"
	}
}

func buildTags(source, memType string) []string {
	return []string{
		"source:" + source,
		"ingested:true",
		"type:" + memType,
	}
}
