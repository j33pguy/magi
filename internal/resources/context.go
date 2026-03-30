package resources

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// Context provides recent and important memories for session auto-injection.
type Context struct {
	DB db.Store
}

// Resource returns the MCP resource definition for context.
func (c *Context) Resource() mcp.Resource {
	return mcp.NewResource(
		"memory://context",
		"Session Context",
		mcp.WithResourceDescription("Recent memories auto-injected at session start"),
		mcp.WithMIMEType("application/json"),
	)
}

// Handle returns recent memories for session context priming.
func (c *Context) Handle(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := os.Getenv("PROJECT_NAME")

	memories, err := c.DB.GetContextMemories(project, 10)
	if err != nil {
		return nil, fmt.Errorf("getting context memories: %w", err)
	}

	if memories == nil {
		memories = []*db.Memory{}
	}

	data, err := marshalJSON(memories, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling context memories: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "memory://context",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
