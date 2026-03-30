package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/internal/db"
)

// Decisions provides architecture decision memories for a project.
type Decisions struct {
	DB db.Store
}

// Template returns the MCP resource template for decision memories.
func (d *Decisions) Template() mcp.ResourceTemplate {
	return mcp.NewResourceTemplate(
		"memory://decisions/{project}",
		"Architecture Decisions",
		mcp.WithTemplateDescription("Architecture decision records for a project"),
		mcp.WithTemplateMIMEType("application/json"),
	)
}

// Handle returns decision-type memories for the requested project.
func (d *Decisions) Handle(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := strings.TrimPrefix(request.Params.URI, "memory://decisions/")

	memories, err := d.DB.ListMemories(&db.MemoryFilter{
		Project: project,
		Type:    "decision",
		Limit:   50,
	})
	if err != nil {
		return nil, fmt.Errorf("listing decisions: %w", err)
	}

	data, err := marshalJSON(memories, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling decisions: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
