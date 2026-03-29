package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/classify"
	"github.com/j33pguy/magi/db"
	"github.com/j33pguy/magi/embeddings"
)

// IndexTurn stores a single conversation turn as a memory.
type IndexTurn struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for index_turn.
func (t *IndexTurn) Tool() mcp.Tool {
	return mcp.NewTool("index_turn",
		mcp.WithDescription("Index a single conversation turn as a memory. Called at the end of significant turns to passively build memory."),
		mcp.WithString("role", mcp.Required(), mcp.Description("Who sent this message"), mcp.Enum("user", "assistant")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The message content")),
		mcp.WithString("project", mcp.Description("Project name/path (auto-detected from PWD if omitted)")),
		mcp.WithString("session_id", mcp.Description("Opaque session identifier for grouping turns")),
	)
}

// Handle processes an index_turn tool call.
func (t *IndexTurn) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return indexTurn(ctx, t.DB, t.Embedder, request)
}

// contentHash returns the first 16 hex chars of sha256(trimmed content).
func contentHash(content string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return fmt.Sprintf("%x", h[:8])
}

// indexTurn is the shared logic for indexing a single turn.
func indexTurn(ctx context.Context, dbClient *db.Client, embedder embeddings.Provider, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	role, err := request.RequireString("role")
	if err != nil {
		return mcp.NewToolResultError("role is required"), nil
	}

	content, err := request.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}

	project := request.GetString("project", "")
	sessionID := request.GetString("session_id", "")

	// Content hash dedup
	hash := contentHash(content)
	existingID, err := dbClient.ExistsWithContentHash(hash)
	if err != nil {
		slog.Warn("content hash check failed, proceeding", "error", err)
	} else if existingID != "" {
		return mcp.NewToolResultText(fmt.Sprintf("Already indexed: %s", existingID)), nil
	}

	// Map role to speaker convention
	speaker := "gilfoyle"
	if role == "user" {
		speaker = "j33p"
	}

	// Auto-classify from content
	c := classify.Infer(content)

	// Generate summary (first 100 chars)
	summary := content
	if len(summary) > 100 {
		summary = summary[:100]
	}

	// Generate embedding
	embedding, err := embedder.Embed(ctx, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating embedding: %v", err)), nil
	}

	memory := &db.Memory{
		Content:    content,
		Summary:    summary,
		Embedding:  embedding,
		Project:    project,
		Type:       "conversation",
		Source:     "claude-code",
		Speaker:    speaker,
		Area:       c.Area,
		SubArea:    c.SubArea,
		TokenCount: len(content) / 4,
	}

	saved, err := dbClient.SaveMemory(memory)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving turn: %v", err)), nil
	}

	// Build tags
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

	if err := dbClient.SetTags(saved.ID, tags); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("setting tags: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Indexed turn %s (role=%s, speaker=%s)", saved.ID, role, speaker)), nil
}
