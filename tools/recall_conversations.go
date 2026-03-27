package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
	"github.com/j33pguy/claude-memory/search"
)

// RecallConversations searches conversation memories using hybrid retrieval.
type RecallConversations struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for recall_conversations.
func (r *RecallConversations) Tool() mcp.Tool {
	return mcp.NewTool("recall_conversations",
		mcp.WithDescription("Search conversation memories using hybrid retrieval (BM25 + semantic vector search). Filters to type=conversation automatically. Set 'min_relevance' to filter out low-quality results."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("channel", mcp.Description("Filter by conversation channel (e.g. 'discord', 'webchat')")),
		mcp.WithNumber("top_k", mcp.Description("Number of results to return (default 5)")),
		mcp.WithNumber("min_relevance", mcp.Description("Minimum relevance score 0.0-1.0 (default 0.0 = no filtering). Results with score below this are excluded.")),
	)
}

// Handle processes a recall_conversations tool call.
func (r *RecallConversations) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	channel := request.GetString("channel", "")
	topK := request.GetInt("top_k", 5)
	minRelevance := request.GetFloat("min_relevance", 0.0)

	var tags []string
	if channel != "" {
		tags = append(tags, "channel:"+channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Visibility: "all",
	}

	resp, err := search.Adaptive(ctx, r.DB, r.Embedder.Embed, query, filter, topK, minRelevance)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search: %v", err)), nil
	}

	if len(resp.Results) == 0 {
		msg := "No matching conversations found."
		if resp.Rewritten {
			msg += fmt.Sprintf(" (also tried rewritten query: %q)", resp.RewrittenQuery)
		}
		return mcp.NewToolResultText(msg), nil
	}

	output, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
