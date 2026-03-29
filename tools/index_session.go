package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/classify"
	"github.com/j33pguy/magi/db"
	"github.com/j33pguy/magi/embeddings"
)

// IndexSession bulk-indexes a completed conversation session.
type IndexSession struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for index_session.
func (s *IndexSession) Tool() mcp.Tool {
	return mcp.NewTool("index_session",
		mcp.WithDescription("Bulk-index a completed conversation session. More efficient than calling index_turn for each message."),
		mcp.WithArray("turns", mcp.Required(), mcp.Description("Array of {role, content} objects representing conversation turns")),
		mcp.WithString("project", mcp.Description("Project name/path")),
		mcp.WithString("session_id", mcp.Description("Session identifier for grouping turns")),
		mcp.WithBoolean("summarize", mcp.Description("If true, also store a rolled-up summary memory (default: false)")),
	)
}

// sessionTurn represents a single turn in the turns array.
type sessionTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Handle processes an index_session tool call.
func (s *IndexSession) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	turnsRaw, ok := args["turns"]
	if !ok {
		return mcp.NewToolResultError("turns is required"), nil
	}

	// Parse turns from the raw interface
	turnsJSON, err := json.Marshal(turnsRaw)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid turns format: %v", err)), nil
	}

	var turns []sessionTurn
	if err := json.Unmarshal(turnsJSON, &turns); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid turns format: %v", err)), nil
	}

	if len(turns) == 0 {
		return mcp.NewToolResultError("turns array is empty"), nil
	}

	project := request.GetString("project", "")
	sessionID := request.GetString("session_id", "")
	summarize := request.GetBool("summarize", false)

	var indexed int
	var skipped int

	for _, turn := range turns {
		if turn.Role == "" || turn.Content == "" {
			skipped++
			continue
		}

		// Content hash dedup
		hash := contentHash(turn.Content)
		existingID, err := s.DB.ExistsWithContentHash(hash)
		if err != nil {
			slog.Warn("content hash check failed, proceeding", "error", err)
		} else if existingID != "" {
			skipped++
			continue
		}

		speaker := "gilfoyle"
		if turn.Role == "user" {
			speaker = "j33p"
		}

		c := classify.Infer(turn.Content)

		summary := turn.Content
		if len(summary) > 100 {
			summary = summary[:100]
		}

		embedding, err := s.Embedder.Embed(ctx, turn.Content)
		if err != nil {
			slog.Warn("embedding failed, skipping turn", "error", err)
			skipped++
			continue
		}

		memory := &db.Memory{
			Content:    turn.Content,
			Summary:    summary,
			Embedding:  embedding,
			Project:    project,
			Type:       "conversation",
			Source:     "claude-code",
			Speaker:    speaker,
			Area:       c.Area,
			SubArea:    c.SubArea,
			TokenCount: len(turn.Content) / 4,
		}

		saved, err := s.DB.SaveMemory(memory)
		if err != nil {
			slog.Warn("saving turn failed", "error", err)
			skipped++
			continue
		}

		tags := []string{"turn", "hash:" + hash, "speaker:" + speaker}
		if sessionID != "" {
			tags = append(tags, "session:"+sessionID)
		}
		if c.Area != "" {
			tags = append(tags, "area:"+c.Area)
		}
		if c.SubArea != "" {
			tags = append(tags, "sub_area:"+c.SubArea)
		}

		if err := s.DB.SetTags(saved.ID, tags); err != nil {
			slog.Warn("setting tags failed", "memory_id", saved.ID, "error", err)
		}

		indexed++
	}

	// Optional summary memory
	if summarize && len(turns) > 0 && sessionID != "" {
		first := turns[0].Content
		last := turns[len(turns)-1].Content

		summaryContent := first
		if len(summaryContent) > 250 {
			summaryContent = summaryContent[:250]
		}
		summaryContent += "\n...\n"
		tail := last
		if len(tail) > 250 {
			tail = tail[:250]
		}
		summaryContent += tail

		// Truncate total to 500 chars
		if len(summaryContent) > 500 {
			summaryContent = summaryContent[:500]
		}

		c := classify.Infer(summaryContent)

		embedding, err := s.Embedder.Embed(ctx, summaryContent)
		if err != nil {
			slog.Warn("summary embedding failed", "error", err)
		} else {
			summaryMem := &db.Memory{
				Content:    summaryContent,
				Summary:    fmt.Sprintf("Session summary: %d turns", len(turns)),
				Embedding:  embedding,
				Project:    project,
				Type:       "conversation_summary",
				Source:     "claude-code",
				Speaker:    "gilfoyle",
				Area:       c.Area,
				SubArea:    c.SubArea,
				TokenCount: len(summaryContent) / 4,
			}

			saved, err := s.DB.SaveMemory(summaryMem)
			if err != nil {
				slog.Warn("saving summary failed", "error", err)
			} else {
				tags := []string{"session:" + sessionID, "summary"}
				if c.Area != "" {
					tags = append(tags, "area:"+c.Area)
				}
				if err := s.DB.SetTags(saved.ID, tags); err != nil {
					slog.Warn("setting summary tags failed", "error", err)
				}
			}
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Indexed %d turns (%d skipped/deduped)", indexed, skipped)), nil
}
