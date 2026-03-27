package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
)

// List browses/filters memories without semantic search.
type List struct {
	DB *db.Client
}

// Tool returns the MCP tool definition for list_memories.
func (l *List) Tool() mcp.Tool {
	return mcp.NewTool("list_memories",
		mcp.WithDescription("Browse and filter stored memories by project, type, or tags. Use recall for semantic search instead."),
		mcp.WithString("project", mcp.Description("Filter by project name")),
		mcp.WithString("type",
			mcp.Description("Filter by memory type"),
			mcp.Enum("memory", "incident", "lesson", "decision", "project_context", "conversation", "audit", "runbook", "preference", "context", "security"),
		),
		mcp.WithArray("tags", mcp.Description("Filter by tags"), mcp.WithStringItems()),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
		mcp.WithNumber("offset", mcp.Description("Pagination offset (default 0)")),
	)
}

// Handle processes a list_memories tool call.
func (l *List) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := &db.MemoryFilter{
		Project: request.GetString("project", ""),
		Type:    request.GetString("type", ""),
		Tags:    request.GetStringSlice("tags", nil),
		Limit:   request.GetInt("limit", 20),
		Offset:  request.GetInt("offset", 0),
	}

	memories, err := l.DB.ListMemories(filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing memories: %v", err)), nil
	}

	if len(memories) == 0 {
		return mcp.NewToolResultText("No memories found matching the filter."), nil
	}

	// Load tags for each memory
	for _, m := range memories {
		tags, err := l.DB.GetTags(m.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("getting tags: %v", err)), nil
		}
		m.Tags = tags
	}

	output, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
