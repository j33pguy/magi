package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
)

// Recall performs hybrid search (BM25 + vector + RRF) over stored memories.
type Recall struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for recall.
func (r *Recall) Tool() mcp.Tool {
	return mcp.NewTool("recall",
		mcp.WithDescription("Search stored memories using hybrid retrieval (BM25 keyword + semantic vector search fused via RRF). Returns the most relevant memories. Use 'projects' to query multiple namespaces at once (e.g. your agent namespace + crew:shared)."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("project", mcp.Description("Filter by a single project/namespace (e.g. 'agent:gilfoyle', 'crew:shared')")),
		mcp.WithArray("projects", mcp.Description("Filter by multiple namespaces — results from any match (e.g. ['agent:dinesh','crew:shared'])"), mcp.WithStringItems()),
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
	projects := request.GetStringSlice("projects", nil)
	memType := request.GetString("type", "")
	tags := request.GetStringSlice("tags", nil)
	topK := request.GetInt("top_k", 5)

	// Generate query embedding for vector leg of hybrid search
	embedding, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating query embedding: %v", err)), nil
	}

	filter := &db.MemoryFilter{
		Project:    project,
		Projects:   projects,
		Type:       memType,
		Tags:       tags,
		Visibility: "all", // MCP callers (Claude Code, Gilfoyle) see all including private
	}

	results, err := r.DB.HybridSearch(embedding, query, filter, topK)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("hybrid search: %v", err)), nil
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
