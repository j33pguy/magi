package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/russseaman/claude-memory/db"
)

// Forget soft-deletes (archives) or permanently deletes a memory.
type Forget struct {
	DB *db.Client
}

// Tool returns the MCP tool definition for forget.
func (f *Forget) Tool() mcp.Tool {
	return mcp.NewTool("forget",
		mcp.WithDescription("Remove a memory. By default soft-deletes (archives) — use permanent=true to hard delete."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Memory ID to forget")),
		mcp.WithBoolean("permanent", mcp.Description("Permanently delete instead of archiving (default false)")),
	)
}

// Handle processes a forget tool call.
func (f *Forget) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := request.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}

	permanent := request.GetBool("permanent", false)

	// Verify the memory exists
	if _, err := f.DB.GetMemory(id); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("memory not found: %v", err)), nil
	}

	if permanent {
		if err := f.DB.DeleteMemory(id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("deleting memory: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Permanently deleted memory %s", id)), nil
	}

	if err := f.DB.ArchiveMemory(id); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("archiving memory: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Archived memory %s", id)), nil
}
