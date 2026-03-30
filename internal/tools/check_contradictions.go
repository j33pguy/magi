package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/internal/contradiction"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// CheckContradictions exposes contradiction detection as a standalone MCP tool.
type CheckContradictions struct {
	DB       db.Store
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for check_contradictions.
func (c *CheckContradictions) Tool() mcp.Tool {
	return mcp.NewTool("check_contradictions",
		mcp.WithDescription("Check if content contradicts any existing memories. Returns potential contradictions with similarity scores and reasons."),
		mcp.WithString("content", mcp.Required(), mcp.Description("The text to check for contradictions against existing memories")),
		mcp.WithString("area", mcp.Description("Filter to this top-level area (work, home, family, homelab, project, meta)")),
		mcp.WithString("sub_area", mcp.Description("Filter to this sub-area")),
		mcp.WithNumber("threshold", mcp.Description("Similarity threshold (0.0-1.0, default 0.85). Higher = stricter matching.")),
	)
}

// Handle processes a check_contradictions tool call.
func (c *CheckContradictions) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := request.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}

	area := request.GetString("area", "")
	subArea := request.GetString("sub_area", "")
	threshold := request.GetFloat("threshold", 0.85)

	if threshold < 0 || threshold > 1 {
		threshold = 0.85
	}

	detector := &contradiction.Detector{Threshold: threshold}
	candidates, err := detector.Check(ctx, c.DB, c.Embedder, content, area, subArea)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("checking contradictions: %v", err)), nil
	}

	if len(candidates) == 0 {
		return mcp.NewToolResultText("No contradictions detected."), nil
	}

	data, err := json.Marshal(map[string]any{
		"contradictions": candidates,
		"count":          len(candidates),
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}
