package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
)

// Recall performs semantic search over stored memories.
type Recall struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for recall.
func (r *Recall) Tool() mcp.Tool {
	return mcp.NewTool("recall",
		mcp.WithDescription("Semantically search stored memories. Returns the most relevant memories based on meaning, not just keywords."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("project", mcp.Description("Filter by project name")),
		mcp.WithString("type",
			mcp.Description("Filter by memory type"),
			mcp.Enum("note", "decision", "audit", "runbook", "preference", "context", "security"),
		),
		mcp.WithArray("tags", mcp.Description("Filter by tags (any match)"), mcp.WithStringItems()),
		mcp.WithNumber("top_k", mcp.Description("Number of results to return (default 5)")),
	)
}

// Handle processes a recall tool call.
func (r *Recall) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	project := request.GetString("project", "")
	memType := request.GetString("type", "")
	tags := request.GetStringSlice("tags", nil)
	topK := request.GetInt("top_k", 5)

	// Generate query embedding
	embedding, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating query embedding: %v", err)), nil
	}

	filter := &db.MemoryFilter{
		Project: project,
		Type:    memType,
		Tags:    tags,
	}

	results, err := r.DB.SearchMemories(embedding, filter, topK)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("searching memories: %v", err)), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("No matching memories found."), nil
	}

	// If a chunk matched, fetch the parent's full content
	for _, result := range results {
		if result.Memory.ParentID != "" {
			parent, err := r.DB.GetMemory(result.Memory.ParentID)
			if err == nil {
				result.Memory.Content = parent.Content
				result.Memory.Tags = parent.Tags
			}
		}
	}

	output, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
