package syncagent

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/j33pguy/magi/internal/project"
)

// Payload is a normalized record sent to MAGI.
type Payload struct {
	Content    string
	Summary    string
	Project    string
	Type       string
	Visibility string
	Tags       []string
	Source     string
	Speaker    string
	SourcePath string
	Hash       string
	Key        string
}

type claudeAdapter struct{}

type claudeTurn struct {
	Role    string
	Content string
}

func (claudeAdapter) Scan(cfg *Config, agent AgentConfig, privacy PrivacyConfig) ([]Payload, error) {
	var payloads []Payload
	maxBytes := privacy.MaxFileSizeKB * 1024
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
			case strings.EqualFold(filepath.Base(path), "CLAUDE.md"), strings.HasSuffix(strings.ToLower(path), ".md"):
				p, ok := markdownPayload(cfg, path, agent, privacy)
				if ok {
					payloads = append(payloads, p)
				}
			case strings.HasSuffix(strings.ToLower(path), ".jsonl"):
				payloads = append(payloads, jsonlPayloads(cfg, path, agent, privacy)...)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	return payloads, nil
}

func markdownPayload(cfg *Config, path string, agent AgentConfig, privacy PrivacyConfig) (Payload, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Payload{}, false
	}
	content := strings.TrimSpace(applyPrivacy(string(data), privacy))
	if content == "" {
		return Payload{}, false
	}
	p := Payload{
		Content:    content,
		Summary:    firstLine(content, 120),
		Project:    detectProjectForPath(path),
		Type:       "project_context",
		Visibility: agent.Visibility,
		Tags:       identityTags(cfg, agent, "project_context"),
		Source:     "magi-sync",
		Speaker:    "system",
		SourcePath: path,
		Hash:       hashBytes(data),
	}
	p.Key = checkpointKey(p)
	return p, true
}

func jsonlPayloads(cfg *Config, path string, agent AgentConfig, privacy PrivacyConfig) []Payload {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var turns []claudeTurn
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		turn := extractTurn(line)
		if strings.TrimSpace(turn.Content) == "" {
			continue
		}
		turn.Content = strings.TrimSpace(applyPrivacy(turn.Content, privacy))
		if turn.Content == "" {
			continue
		}
		turns = append(turns, turn)
		if len(turns) >= 20 {
			break
		}
	}
	if len(turns) == 0 {
		return nil
	}
	project := detectProjectForPath(path)
	content := formatTurns(turns)
	summary := summarizeTurns(turns, 120)
	tags := identityTags(cfg, agent, "conversation_summary")
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
		Speaker:    summarySpeaker(turns),
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
		ptags = append(ptags, "speaker:"+normalizeRole(turn.Role), fmt.Sprintf("turn_index:%d", i))
		p := Payload{
			Content:    turnContent,
			Summary:    fmt.Sprintf("%s turn %d: %s", normalizeRole(turn.Role), i+1, firstLine(turnContent, 80)),
			Project:    project,
			Type:       "conversation",
			Visibility: agent.Visibility,
			Tags:       ptags,
			Source:     "magi-sync",
			Speaker:    normalizeRole(turn.Role),
			SourcePath: path,
			Hash:       hashString(fmt.Sprintf("%s|%d|%s", path, i, turnContent)),
		}
		p.Key = checkpointKey(p)
		payloads = append(payloads, p)
	}
	return payloads
}

func extractTurn(line string) claudeTurn {
	var v any
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		return claudeTurn{Role: "assistant", Content: line}
	}
	role := extractRole(v)
	parts := collectText(v)
	if len(parts) == 0 {
		return claudeTurn{Role: role, Content: line}
	}
	return claudeTurn{
		Role:    role,
		Content: strings.Join(parts, "\n"),
	}
}

func collectText(v any) []string {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		var out []string
		for _, item := range x {
			out = append(out, collectText(item)...)
		}
		return dedupeStrings(out)
	case map[string]any:
		priority := []string{"content", "text", "message", "input", "output", "summary"}
		ignore := []string{"type", "role", "id", "uuid", "timestamp", "created_at", "updated_at"}
		var out []string
		for _, key := range priority {
			if child, ok := x[key]; ok {
				out = append(out, collectText(child)...)
			}
		}
		for key, child := range x {
			if contains(priority, key) || contains(ignore, key) {
				continue
			}
			out = append(out, collectText(child)...)
		}
		return dedupeStrings(out)
	default:
		return nil
	}
}

func extractRole(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, key := range []string{"role", "speaker", "author"} {
			if val, ok := x[key].(string); ok {
				return normalizeRole(val)
			}
		}
		for _, key := range []string{"message", "content", "input", "output"} {
			if child, ok := x[key]; ok {
				if role := extractRole(child); role != "" {
					return role
				}
			}
		}
	case []any:
		for _, child := range x {
			if role := extractRole(child); role != "" {
				return role
			}
		}
	}
	return "assistant"
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "human", "user":
		return "user"
	case "assistant", "claude", "model":
		return "assistant"
	case "system":
		return "system"
	default:
		if role == "" {
			return "assistant"
		}
		return role
	}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func formatTurns(turns []claudeTurn) string {
	lines := make([]string, 0, len(turns))
	for _, turn := range turns {
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", normalizeRole(turn.Role), content))
	}
	return strings.Join(lines, "\n")
}

func summarizeTurns(turns []claudeTurn, max int) string {
	if len(turns) == 0 {
		return ""
	}
	first := ""
	last := ""
	for _, turn := range turns {
		if strings.TrimSpace(turn.Content) == "" {
			continue
		}
		if first == "" {
			first = fmt.Sprintf("%s: %s", normalizeRole(turn.Role), firstLine(turn.Content, 60))
		}
		last = fmt.Sprintf("%s: %s", normalizeRole(turn.Role), firstLine(turn.Content, 60))
	}
	summary := first
	if last != "" && last != first {
		summary = first + " -> " + last
	}
	if len(summary) > max {
		return summary[:max]
	}
	return summary
}

func summarySpeaker(turns []claudeTurn) string {
	if len(turns) == 0 {
		return "assistant"
	}
	last := normalizeRole(turns[len(turns)-1].Role)
	if last == "" {
		return "assistant"
	}
	return last
}

func rolesSeen(turns []claudeTurn) []string {
	seen := map[string]struct{}{}
	var roles []string
	for _, turn := range turns {
		role := normalizeRole(turn.Role)
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx != -1 {
		s = s[:idx]
	}
	if len(s) > max {
		return s[:max]
	}
	return s
}

func detectProjectForPath(path string) string {
	dir := filepath.Dir(path)
	for {
		if dir == "." || dir == "/" || dir == "" {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return project.DetectProject(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return project.DetectProject(filepath.Dir(path))
}

func identityTags(cfg *Config, agent AgentConfig, memType string) []string {
	tags := []string{
		"source:claude_sync",
		"agent:" + agent.Type,
		"agent_name:" + agent.Name,
		"type:" + memType,
		"visibility:" + agent.Visibility,
	}
	if cfg != nil {
		if cfg.Machine.User != "" {
			tags = append(tags, "user:"+cfg.Machine.User)
		}
		if cfg.Machine.ID != "" {
			tags = append(tags, "machine:"+cfg.Machine.ID)
		}
		if cfg.Machine.User != "" && cfg.Machine.ID != "" {
			tags = append(tags, "identity:"+cfg.Machine.User+"."+cfg.Machine.ID)
		}
	}
	if agent.Owner != "" {
		tags = append(tags, "owner:"+agent.Owner)
	}
	for _, viewer := range agent.Viewers {
		viewer = strings.TrimSpace(viewer)
		if viewer == "" {
			continue
		}
		tags = append(tags, "viewer:"+viewer)
	}
	for _, group := range agent.ViewerGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		tags = append(tags, "viewer_group:"+group)
	}
	return dedupeStrings(tags)
}

func checkpointKey(p Payload) string {
	if p.Key != "" {
		return p.Key
	}
	return p.SourcePath + "|" + p.Type + "|" + p.Hash + "|" + p.Speaker
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
