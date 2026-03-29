package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/search"
)

// RecallLessons searches only lesson-type memories (hard-won knowledge, gotchas).
type RecallLessons struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for recall_lessons.
func (r *RecallLessons) Tool() mcp.Tool {
	return mcp.NewTool("recall_lessons",
		mcp.WithDescription("Search lesson memories — hard-won knowledge, gotchas, and things learned the hard way. Automatically filters to type=lesson. Use this before making changes to check for known pitfalls."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("project", mcp.Description("Filter by project/namespace")),
		mcp.WithArray("projects", mcp.Description("Filter by multiple namespaces"), mcp.WithStringItems()),
		mcp.WithArray("tags", mcp.Description("Filter by tags (any match)"), mcp.WithStringItems()),
		mcp.WithNumber("top_k", mcp.Description("Number of results to return (default 5)")),
		mcp.WithNumber("recency_decay", mcp.Description("Exponential decay rate for recency weighting (default 0.0 = disabled). Recommended: 0.01.")),
	)
}

// Handle processes a recall_lessons tool call.
func (r *RecallLessons) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	project := request.GetString("project", "")
	projects := request.GetStringSlice("projects", nil)
	tags := request.GetStringSlice("tags", nil)
	topK := request.GetInt("top_k", 5)
	recencyDecay := request.GetFloat("recency_decay", 0.0)

	embedding, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating query embedding: %v", err)), nil
	}

	filter := &db.MemoryFilter{
		Project:    project,
		Projects:   projects,
		Type:       "lesson",
		Tags:       tags,
		Visibility: "all",
	}

	results, err := r.DB.HybridSearch(embedding, query, filter, topK)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("hybrid search: %v", err)), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("No matching lessons found."), nil
	}

	for _, result := range results {
		if result.Memory.ParentID != "" {
			parent, err := r.DB.GetMemory(result.Memory.ParentID)
			if err == nil {
				result.Memory.Content = parent.Content
				result.Memory.Tags = parent.Tags
			}
		}
	}

	search.ApplyRecencyWeighting(results, recencyDecay)

	output, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
