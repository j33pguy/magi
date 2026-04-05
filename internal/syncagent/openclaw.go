package syncagent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type openclawAdapter struct{}

// openclawSession represents the session header line in an OpenClaw JSONL.
type openclawSession struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Version string `json:"version"`
	CWD     string `json:"cwd"`
}

// openclawEntry represents a single line in an OpenClaw session JSONL.
type openclawEntry struct {
	Type       string          `json:"type"`
	CustomType string          `json:"customType,omitempty"`
	ID         string          `json:"id,omitempty"`
	Timestamp  string          `json:"timestamp,omitempty"`
	Message    json.RawMessage `json:"message,omitempty"`
}

// openclawMessage represents a message within an OpenClaw entry.
type openclawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func (openclawAdapter) Scan(cfg *Config, agent AgentConfig, privacy PrivacyConfig) ([]Payload, error) {
	var payloads []Payload
	maxKB := privacy.MaxFileSizeKB
	if maxKB <= 0 {
		maxKB = 512
	}
	maxBytes := maxKB * 1024

	for _, root := range agent.Paths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			if !shouldInclude(rel, agent.Include, agent.Exclude) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.Size() > maxBytes {
				return nil
			}

			switch {
			case strings.HasSuffix(strings.ToLower(path), ".jsonl"):
				payloads = append(payloads, openclawSessionPayloads(cfg, path, agent, privacy)...)
			case strings.HasSuffix(strings.ToLower(path), ".md"):
				p, ok := openclawMarkdownPayload(cfg, path, agent, privacy)
				if ok {
					payloads = append(payloads, p)
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	return payloads, nil
}

func openclawSessionPayloads(cfg *Config, path string, agent AgentConfig, privacy PrivacyConfig) []Payload {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var sessionID string
	var turns []claudeTurn
	scanner := bufio.NewScanner(f)
	// Increase buffer for large lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry openclawEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "session":
			var sess openclawSession
			if err := json.Unmarshal([]byte(line), &sess); err == nil {
				sessionID = sess.ID
			}
		case "message":
			if entry.Message == nil {
				continue
			}
			var msg openclawMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			content := extractOpenClawContent(msg.Content)
			content = strings.TrimSpace(applyPrivacy(content, privacy))
			if content == "" {
				continue
			}
			role := normalizeOpenClawRole(msg.Role)
			turns = append(turns, claudeTurn{
				Role:    role,
				Content: content,
			})
		}

		// Cap at 20 turns to avoid huge payloads
		if len(turns) >= 20 {
			break
		}
	}

	if len(turns) == 0 {
		return nil
	}

	project := detectOpenClawProject(path)
	speaker := "openclaw"
	if sessionID != "" {
		speaker = "openclaw:" + sessionID[:8] // short session prefix
	}

	content := formatTurns(turns)
	summary := summarizeTurns(turns, 120)
	tags := identityTags(cfg, agent, "conversation_summary")
	tags = append(tags, "source:openclaw")
	if sessionID != "" {
		tags = append(tags, "session:"+sessionID)
	}
	for _, role := range rolesSeen(turns) {
		tags = append(tags, "speaker:"+role)
	}

	summaryPayload := Payload{
		Content:    content,
		Summary:    summary,
		Project:    project,
		Type:       "conversation_summary",
		Visibility: agent.Visibility,
		Tags:       tags,
		Source:     "magi-sync",
		Speaker:    speaker,
		SourcePath: path,
		Hash:       hashString(content),
	}
	summaryPayload.Key = checkpointKey(summaryPayload)

	payloads := []Payload{summaryPayload}

	for i, turn := range turns {
		turnContent := strings.TrimSpace(turn.Content)
		if turnContent == "" {
			continue
		}
		ptags := identityTags(cfg, agent, "conversation")
		ptags = append(ptags, "speaker:"+turn.Role, fmt.Sprintf("turn_index:%d", i), "source:openclaw")
		if sessionID != "" {
			ptags = append(ptags, "session:"+sessionID)
		}
		p := Payload{
			Content:    turnContent,
			Summary:    fmt.Sprintf("%s turn %d: %s", turn.Role, i+1, firstLine(turnContent, 80)),
			Project:    project,
			Type:       "conversation",
			Visibility: agent.Visibility,
			Tags:       ptags,
			Source:     "magi-sync",
			Speaker:    normalizeOpenClawRole(turn.Role),
			SourcePath: path,
			Hash:       hashString(turnContent),
		}
		p.Key = checkpointKey(p)
		payloads = append(payloads, p)
	}

	return payloads
}

func openclawMarkdownPayload(cfg *Config, path string, agent AgentConfig, privacy PrivacyConfig) (Payload, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Payload{}, false
	}
	content := strings.TrimSpace(applyPrivacy(string(data), privacy))
	if content == "" {
		return Payload{}, false
	}

	project := detectOpenClawProject(path)
	fileName := filepath.Base(path)

	// Determine speaker based on the file
	speaker := "openclaw"
	switch strings.ToUpper(fileName) {
	case "SOUL.MD", "IDENTITY.MD", "AGENTS.MD":
		speaker = "openclaw-agent"
	case "USER.MD":
		speaker = "user"
	case "HEARTBEAT.MD", "TOOLS.MD":
		speaker = "openclaw-system"
	}

	// Files in memory/ directory are agent memory
	if strings.Contains(filepath.ToSlash(path), "/memory/") {
		speaker = "openclaw-agent"
	}

	p := Payload{
		Content:    content,
		Summary:    firstLine(content, 120),
		Project:    project,
		Type:       "project_context",
		Visibility: agent.Visibility,
		Tags:       identityTags(cfg, agent, "project_context"),
		Source:     "magi-sync",
		Speaker:    speaker,
		SourcePath: path,
		Hash:       hashBytes(data),
	}
	p.Key = checkpointKey(p)
	return p, true
}

// extractOpenClawContent extracts text from OpenClaw message content.
// Content can be a string or an array of content blocks.
func extractOpenClawContent(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try array of content blocks
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, block := range blocks {
		var obj map[string]interface{}
		if err := json.Unmarshal(block, &obj); err != nil {
			continue
		}
		// Extract text from text blocks
		if text, ok := obj["text"].(string); ok && text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeOpenClawRole(role string) string {
	switch strings.ToLower(role) {
	case "user", "human":
		return "user"
	case "assistant", "ai":
		return "assistant"
	case "system":
		return "system"
	case "toolresult", "tool_result", "tool":
		return "tool"
	default:
		return role
	}
}

// detectOpenClawProject detects the project name from an OpenClaw path.
// Structure: ~/.openclaw/workspace/ → "openclaw-workspace"
// Structure: ~/.openclaw/agents/<name>/sessions/ → agent name
func detectOpenClawProject(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		// ~/.openclaw/agents/<agent-name>/sessions/<file>
		if part == "agents" && i+1 < len(parts) {
			return "openclaw-" + parts[i+1]
		}
		// ~/.openclaw/workspace/ files
		if part == "workspace" {
			return "openclaw-workspace"
		}
	}
	return "openclaw"
}
