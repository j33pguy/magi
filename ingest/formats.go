// Package ingest detects and parses conversation exports from Grok, ChatGPT, and plain text.
package ingest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Format represents the detected format of input data.
type Format string

const (
	FormatGrok      Format = "grok"
	FormatChatGPT   Format = "chatgpt"
	FormatPlainText Format = "plaintext"
	FormatMarkdown  Format = "markdown"
	FormatGitRepo   Format = "gitrepo"
	FormatUnknown   Format = "unknown"
)

// Turn represents a single conversation turn.
type Turn struct {
	Role    string     // "user" | "assistant" | "system"
	Content string
	Time    *time.Time
}

// ParsedConversation is the result of parsing any supported format.
type ParsedConversation struct {
	Format Format
	Turns  []Turn
	Title  string
	Source string // "grok", "chatgpt", etc.
}

// MaxInputSize is the maximum allowed input size (10 MB).
const MaxInputSize = 10 * 1024 * 1024

// Detect auto-detects the format of the input data.
func Detect(data []byte) Format {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 {
		return FormatUnknown
	}

	// Try JSON formats first
	if trimmed[0] == '[' || trimmed[0] == '{' {
		if isGrokFormat(data) {
			return FormatGrok
		}
		if isChatGPTFormat(data) {
			return FormatChatGPT
		}
	}

	// Plain text with role markers
	if plainTextRolePattern.MatchString(trimmed) {
		return FormatPlainText
	}

	// Markdown headings
	if strings.HasPrefix(trimmed, "#") {
		return FormatMarkdown
	}

	return FormatUnknown
}

var plainTextRolePattern = regexp.MustCompile(`(?m)^(Human|User|Assistant):\s`)

// Parse parses input data into a ParsedConversation.
func Parse(data []byte) (*ParsedConversation, error) {
	if len(data) > MaxInputSize {
		return nil, fmt.Errorf("input too large: %d bytes (max %d)", len(data), MaxInputSize)
	}

	format := Detect(data)
	switch format {
	case FormatGrok:
		return parseGrok(data)
	case FormatChatGPT:
		return parseChatGPT(data)
	case FormatPlainText:
		return parsePlainText(data)
	case FormatMarkdown:
		return parseMarkdown(data)
	default:
		return nil, fmt.Errorf("unrecognized format")
	}
}

// --- Grok ---

// grokExport is the top-level Grok JSON structure.
type grokExport struct {
	ConversationID string    `json:"conversation_id"`
	Title          string    `json:"title"`
	CreateTime     string    `json:"create_time"`
	Turns          []grokTurn `json:"turns"`
}

type grokTurn struct {
	Author  grokAuthor  `json:"author"`
	Content grokContent `json:"content"`
}

type grokAuthor struct {
	Role string `json:"role"`
}

type grokContent struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}

func isGrokFormat(data []byte) bool {
	// Try array of conversations
	var arr []json.RawMessage
	if json.Unmarshal(data, &arr) != nil || len(arr) == 0 {
		return false
	}
	// Peek at first element
	var probe struct {
		ConversationID string `json:"conversation_id"`
		Turns          []struct {
			Author struct {
				Role string `json:"role"`
			} `json:"author"`
		} `json:"turns"`
	}
	if json.Unmarshal(arr[0], &probe) != nil {
		return false
	}
	return probe.ConversationID != "" && len(probe.Turns) > 0 && probe.Turns[0].Author.Role != ""
}

func parseGrok(data []byte) (*ParsedConversation, error) {
	var convos []grokExport
	if err := json.Unmarshal(data, &convos); err != nil {
		return nil, fmt.Errorf("parsing Grok JSON: %w", err)
	}

	conv := &ParsedConversation{
		Format: FormatGrok,
		Source: "grok",
	}

	for _, c := range convos {
		if conv.Title == "" {
			conv.Title = c.Title
		}
		for _, t := range c.Turns {
			role := normalizeRole(t.Author.Role)
			content := strings.Join(t.Content.Parts, "\n")
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			conv.Turns = append(conv.Turns, Turn{Role: role, Content: content})
		}
	}

	return conv, nil
}

// --- ChatGPT ---

type chatGPTExport struct {
	ID      string                       `json:"id"`
	Title   string                       `json:"title"`
	Mapping map[string]chatGPTMappingNode `json:"mapping"`
}

type chatGPTMappingNode struct {
	ID       string          `json:"id"`
	Parent   string          `json:"parent"`
	Children []string        `json:"children"`
	Message  *chatGPTMessage `json:"message"`
}

type chatGPTMessage struct {
	ID      string            `json:"id"`
	Author  chatGPTAuthor     `json:"author"`
	Content chatGPTMsgContent `json:"content"`
}

type chatGPTAuthor struct {
	Role string `json:"role"`
}

type chatGPTMsgContent struct {
	ContentType string   `json:"content_type"`
	Parts       []any    `json:"parts"`
}

func isChatGPTFormat(data []byte) bool {
	var arr []json.RawMessage
	if json.Unmarshal(data, &arr) != nil || len(arr) == 0 {
		return false
	}
	var probe struct {
		Mapping map[string]json.RawMessage `json:"mapping"`
	}
	if json.Unmarshal(arr[0], &probe) != nil {
		return false
	}
	return len(probe.Mapping) > 0
}

func parseChatGPT(data []byte) (*ParsedConversation, error) {
	var convos []chatGPTExport
	if err := json.Unmarshal(data, &convos); err != nil {
		return nil, fmt.Errorf("parsing ChatGPT JSON: %w", err)
	}

	conv := &ParsedConversation{
		Format: FormatChatGPT,
		Source: "chatgpt",
	}

	for _, c := range convos {
		if conv.Title == "" {
			conv.Title = c.Title
		}
		turns := flattenChatGPTMapping(c.Mapping)
		conv.Turns = append(conv.Turns, turns...)
	}

	return conv, nil
}

// flattenChatGPTMapping walks the ChatGPT mapping tree to extract turns in order.
func flattenChatGPTMapping(mapping map[string]chatGPTMappingNode) []Turn {
	// Find root (no parent)
	var rootID string
	for id, node := range mapping {
		if node.Parent == "" {
			rootID = id
			break
		}
	}
	if rootID == "" {
		return nil
	}

	var turns []Turn
	var walk func(id string)
	walk = func(id string) {
		node, ok := mapping[id]
		if !ok {
			return
		}
		if node.Message != nil {
			role := normalizeRole(node.Message.Author.Role)
			if role == "user" || role == "assistant" {
				content := extractChatGPTParts(node.Message.Content.Parts)
				content = strings.TrimSpace(content)
				if content != "" {
					turns = append(turns, Turn{Role: role, Content: content})
				}
			}
		}
		for _, childID := range node.Children {
			walk(childID)
		}
	}
	walk(rootID)
	return turns
}

func extractChatGPTParts(parts []any) string {
	var texts []string
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			texts = append(texts, v)
		case map[string]any:
			// Some parts are objects (images, etc.) — skip
		}
	}
	return strings.Join(texts, "\n")
}

// --- Plain text ---

func parsePlainText(data []byte) (*ParsedConversation, error) {
	conv := &ParsedConversation{
		Format: FormatPlainText,
		Source: "plaintext",
	}

	lines := strings.Split(string(data), "\n")
	var currentRole string
	var currentContent strings.Builder

	flush := func() {
		if currentRole != "" {
			content := strings.TrimSpace(currentContent.String())
			if content != "" {
				conv.Turns = append(conv.Turns, Turn{Role: currentRole, Content: content})
			}
		}
		currentContent.Reset()
	}

	for _, line := range lines {
		if m := plainTextRoleMatch(line); m != "" {
			flush()
			currentRole = m
			// Content after the "Role:" prefix
			idx := strings.Index(line, ":")
			rest := strings.TrimSpace(line[idx+1:])
			if rest != "" {
				currentContent.WriteString(rest)
			}
		} else {
			if currentRole != "" {
				if currentContent.Len() > 0 {
					currentContent.WriteString("\n")
				}
				currentContent.WriteString(line)
			}
		}
	}
	flush()

	return conv, nil
}

func plainTextRoleMatch(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"Human:", "User:"} {
		if strings.HasPrefix(trimmed, prefix) {
			return "user"
		}
	}
	if strings.HasPrefix(trimmed, "Assistant:") {
		return "assistant"
	}
	return ""
}

// --- Markdown ---

func parseMarkdown(data []byte) (*ParsedConversation, error) {
	conv := &ParsedConversation{
		Format: FormatMarkdown,
		Source: "markdown",
	}

	// Treat the whole markdown as a single user turn
	content := strings.TrimSpace(string(data))
	if content != "" {
		conv.Turns = append(conv.Turns, Turn{Role: "user", Content: content})
	}

	return conv, nil
}

// --- Helpers ---

func normalizeRole(role string) string {
	switch strings.ToLower(role) {
	case "user", "human":
		return "user"
	case "assistant", "model", "bot", "grok", "chatgpt":
		return "assistant"
	case "system":
		return "system"
	default:
		return role
	}
}
