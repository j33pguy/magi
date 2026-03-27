package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
)

// Update modifies an existing memory's content or metadata, re-embedding if content changed.
type Update struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for update_memory.
func (u *Update) Tool() mcp.Tool {
	return mcp.NewTool("update_memory",
		mcp.WithDescription("Update an existing memory's content, metadata, or tags. Re-embeds automatically if content changes."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Memory ID to update")),
		mcp.WithString("content", mcp.Description("New content (triggers re-embedding)")),
		mcp.WithString("summary", mcp.Description("New summary")),
		mcp.WithString("type",
			mcp.Description("New memory type"),
			mcp.Enum("memory", "incident", "lesson", "decision", "project_context", "conversation", "audit", "runbook", "preference", "context", "security"),
		),
		mcp.WithArray("tags", mcp.Description("Replace tags with these"), mcp.WithStringItems()),
	)
}

// Handle processes an update_memory tool call.
func (u *Update) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := request.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	existing, err := u.DB.GetMemory(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("memory not found: %v", err)), nil
	}

	// Apply updates
	args := request.GetArguments()

	if content, ok := args["content"].(string); ok && content != "" {
		existing.Content = content
		existing.TokenCount = len(content) / 4

		// Re-embed
		embedding, err := u.Embedder.Embed(ctx, content)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("re-embedding: %v", err)), nil
		}
		existing.Embedding = embedding
	}

	if summary, ok := args["summary"].(string); ok {
		existing.Summary = summary
	}

	if memType, ok := args["type"].(string); ok && memType != "" {
		existing.Type = memType
	}

	if err := u.DB.UpdateMemory(existing); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("updating memory: %v", err)), nil
	}

	// Update tags if provided
	if tagsRaw, ok := args["tags"]; ok {
		if tagsSlice, ok := tagsRaw.([]any); ok {
			tags := make([]string, len(tagsSlice))
			for i, t := range tagsSlice {
				tags[i] = fmt.Sprintf("%v", t)
			}
			if err := u.DB.SetTags(id, tags); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("updating tags: %v", err)), nil
			}
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Updated memory %s", id)), nil
}
