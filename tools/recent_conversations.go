package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
)

// RecentConversations lists recent conversation summaries with optional filtering.
type RecentConversations struct {
	DB *db.Client
}

// Tool returns the MCP tool definition for recent_conversations.
func (r *RecentConversations) Tool() mcp.Tool {
	return mcp.NewTool("recent_conversations",
		mcp.WithDescription("List recent conversation summaries across channels. Use recall_conversations for semantic search instead."),
		mcp.WithString("channel", mcp.Description("Filter by channel (e.g. 'claude-code', 'slack')")),
		mcp.WithString("since", mcp.Description("Only return conversations after this timestamp (RFC3339)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
	)
}

// Handle processes a recent_conversations tool call.
func (r *RecentConversations) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channel := request.GetString("channel", "")
	sinceStr := request.GetString("since", "")
	limit := request.GetInt("limit", 10)

	tags := []string{"conversation"}
	if channel != "" {
		tags = append(tags, "channel:"+channel)
	}

	filter := &db.MemoryFilter{
		Type:       "conversation",
		Tags:       tags,
		Limit:      limit,
		Visibility: "all",
	}

	var sinceTime time.Time
	if sinceStr != "" {
		var err error
		sinceTime, err = time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return mcp.NewToolResultError("invalid since timestamp, use RFC3339 format"), nil
		}
		filter.Limit = limit * 5 // over-fetch to account for filtering
	}

	memories, err := r.DB.ListMemories(filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing conversations: %v", err)), nil
	}

	// Load tags
	for _, m := range memories {
		tags, err := r.DB.GetTags(m.ID)
		if err != nil {
			continue
		}
		m.Tags = tags
	}

	// Filter by since if provided
	if !sinceTime.IsZero() {
		filtered := make([]*db.Memory, 0, len(memories))
		for _, m := range memories {
			created, err := time.Parse(time.DateTime, m.CreatedAt)
			if err != nil {
				continue
			}
			if created.After(sinceTime) || created.Equal(sinceTime) {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
		if len(memories) > limit {
			memories = memories[:limit]
		}
	}

	if len(memories) == 0 {
		return mcp.NewToolResultText("No recent conversations found."), nil
	}

	output, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
