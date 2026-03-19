package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/russseaman/claude-memory/db"
)

// Preferences provides all user preference memories.
type Preferences struct {
	DB *db.Client
}

// Resource returns the MCP resource definition for preferences.
func (p *Preferences) Resource() mcp.Resource {
	return mcp.NewResource(
		"memory://preferences",
		"User Preferences",
		mcp.WithResourceDescription("All user preference memories across projects"),
		mcp.WithMIMEType("application/json"),
	)
}

// Handle returns all preference-type memories.
func (p *Preferences) Handle(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	memories, err := p.DB.ListMemories(&db.MemoryFilter{
		Type:  "preference",
		Limit: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("listing preferences: %w", err)
	}

	data, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling preferences: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "memory://preferences",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
