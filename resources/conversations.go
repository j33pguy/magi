package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/db"
)

// RecentConversations provides recent conversation summaries across all channels.
type RecentConversations struct {
	DB *db.Client
}

// Resource returns the MCP resource definition for recent conversations.
func (r *RecentConversations) Resource() mcp.Resource {
	return mcp.NewResource(
		"memory://conversations/recent",
		"Recent Conversations",
		mcp.WithResourceDescription("Last 5 conversation summaries across all channels for cross-channel context"),
		mcp.WithMIMEType("application/json"),
	)
}

// Handle returns recent conversation summaries for session context priming.
func (r *RecentConversations) Handle(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	memories, err := r.DB.ListMemories(&db.MemoryFilter{
		Type:       "conversation",
		Tags:       []string{"conversation"},
		Limit:      5,
		Visibility: "all",
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent conversations: %w", err)
	}

	for _, m := range memories {
		tags, err := r.DB.GetTags(m.ID)
		if err != nil {
			continue
		}
		m.Tags = tags
	}

	if memories == nil {
		memories = []*db.Memory{}
	}

	data, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling conversations: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "memory://conversations/recent",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
