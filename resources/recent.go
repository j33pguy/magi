// Package resources implements MCP resource handlers for passive context.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
)

// Recent provides the 10 most recent memories for a project.
type Recent struct {
	DB *db.Client
}

// Template returns the MCP resource template for recent memories.
func (r *Recent) Template() mcp.ResourceTemplate {
	return mcp.NewResourceTemplate(
		"memory://recent/{project}",
		"Recent Memories",
		mcp.WithTemplateDescription("10 most recent memories for a project"),
		mcp.WithTemplateMIMEType("application/json"),
	)
}

// Handle returns the recent memories for the requested project.
func (r *Recent) Handle(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := extractParam(request.Params.URI, "memory://recent/")

	memories, err := r.DB.ListMemories(&db.MemoryFilter{
		Project: project,
		Limit:   10,
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent memories: %w", err)
	}

	data, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling memories: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func extractParam(uri, prefix string) string {
	return strings.TrimPrefix(uri, prefix)
}
