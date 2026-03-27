package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
)

// RecallConversations searches conversation history using hybrid retrieval.
type RecallConversations struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for recall_conversations.
func (r *RecallConversations) Tool() mcp.Tool {
	return mcp.NewTool("recall_conversations",
		mcp.WithDescription("Search conversation history using hybrid retrieval (BM25 + semantic). Find past conversations by topic, decision, or natural language query."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("channel", mcp.Description("Filter by channel (e.g. 'claude-code', 'slack')")),
		mcp.WithNumber("limit", mcp.Description("Number of results to return (default 5)")),
	)
}

// Handle processes a recall_conversations tool call.
func (r *RecallConversations) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	channel := request.GetString("channel", "")
	limit := request.GetInt("limit", 5)

	embedding, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating query embedding: %v", err)), nil
	}

	var tags []string
	if channel != "" {
		tags = append(tags, "channel:"+channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Visibility: "all",
	}

	results, err := r.DB.HybridSearch(embedding, query, filter, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("searching conversations: %v", err)), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("No matching conversations found."), nil
	}

	output, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
