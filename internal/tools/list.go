package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/j33pguy/magi/internal/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// List browses/filters memories without semantic search.
type List struct {
	DB db.Store
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
		mcp.WithString("speaker",
			mcp.Description("Filter by speaker: user, assistant, agent, system"),
			mcp.Enum("user", "assistant", "agent", "system"),
		),
		mcp.WithString("area",
			mcp.Description("Filter by area: work, home, family, homelab, project, meta"),
			mcp.Enum("work", "home", "family", "homelab", "project", "meta"),
		),
		mcp.WithString("sub_area", mcp.Description("Filter by sub-area (e.g. power-platform, proxmox, magi)")),
		mcp.WithString("after", mcp.Description("Only memories created after this time. Relative (7d, 2w, 1m, 1y) or absolute (2006-01-02, RFC3339).")),
		mcp.WithString("before", mcp.Description("Only memories created before this time. Relative (7d, 2w, 1m, 1y) or absolute (2006-01-02, RFC3339).")),
	)
}

// Handle processes a list_memories tool call.
func (l *List) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	afterTime, err := ParseTimeParam(request.GetString("after", ""))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid 'after': %v", err)), nil
	}
	beforeTime, err := ParseTimeParam(request.GetString("before", ""))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid 'before': %v", err)), nil
	}

	filter := &db.MemoryFilter{
		Project:    request.GetString("project", ""),
		Type:       request.GetString("type", ""),
		Tags:       request.GetStringSlice("tags", nil),
		Limit:      request.GetInt("limit", 20),
		Offset:     request.GetInt("offset", 0),
		Speaker:    request.GetString("speaker", ""),
		Area:       request.GetString("area", ""),
		SubArea:    request.GetString("sub_area", ""),
		AfterTime:  afterTime,
		BeforeTime: beforeTime,
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
