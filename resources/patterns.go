package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/claude-memory/db"
)

// Patterns provides all detected behavioral patterns.
type Patterns struct {
	DB *db.Client
}

// Resource returns the MCP resource definition for patterns.
func (p *Patterns) Resource() mcp.Resource {
	return mcp.NewResource(
		"memory://patterns",
		"Behavioral Patterns",
		mcp.WithResourceDescription("Auto-detected behavioral patterns and preferences"),
		mcp.WithMIMEType("application/json"),
	)
}

// Handle returns all pattern memories ordered by creation time (newest first).
func (p *Patterns) Handle(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	memories, err := p.DB.ListMemories(&db.MemoryFilter{
		Tags:       []string{"pattern"},
		Limit:      100,
		Visibility: "all",
	})
	if err != nil {
		return nil, fmt.Errorf("listing patterns: %w", err)
	}

	if memories == nil {
		memories = []*db.Memory{}
	}

	data, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling patterns: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "memory://patterns",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
