package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// LinkMemories creates a directed link between two memories.
type LinkMemories struct {
	DB *db.Client
}

// Tool returns the MCP tool definition for link_memories.
func (l *LinkMemories) Tool() mcp.Tool {
	return mcp.NewTool("link_memories",
		mcp.WithDescription("Create a directed relationship between two memories (e.g. caused_by, led_to, supersedes)."),
		mcp.WithString("from_id", mcp.Required(), mcp.Description("Source memory ID")),
		mcp.WithString("to_id", mcp.Required(), mcp.Description("Target memory ID")),
		mcp.WithString("relation", mcp.Required(), mcp.Description("One of: caused_by, led_to, related_to, supersedes, part_of, contradicts")),
		mcp.WithNumber("weight", mcp.Description("Relationship strength 0.0–1.0 (default 1.0)")),
	)
}

// Handle processes a link_memories tool call.
func (l *LinkMemories) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fromID, err := request.RequireString("from_id")
	if err != nil {
		return mcp.NewToolResultError("from_id is required"), nil
	}
	toID, err := request.RequireString("to_id")
	if err != nil {
		return mcp.NewToolResultError("to_id is required"), nil
	}
	relation, err := request.RequireString("relation")
	if err != nil {
		return mcp.NewToolResultError("relation is required"), nil
	}

	weight := request.GetFloat("weight", 1.0)

	// Verify both memories exist
	if _, err := l.DB.GetMemory(fromID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("source memory not found: %v", err)), nil
	}
	if _, err := l.DB.GetMemory(toID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("target memory not found: %v", err)), nil
	}

	link, err := l.DB.CreateLink(ctx, fromID, toID, relation, weight, false)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("creating link: %v", err)), nil
	}

	out, _ := json.MarshalIndent(link, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// GetRelated retrieves memories related to a given memory via graph traversal.
type GetRelated struct {
	DB *db.Client
}

// Tool returns the MCP tool definition for get_related.
func (g *GetRelated) Tool() mcp.Tool {
	return mcp.NewTool("get_related",
		mcp.WithDescription("Get memories related to a given memory via link traversal."),
		mcp.WithString("memory_id", mcp.Required(), mcp.Description("Memory ID to find relations for")),
		mcp.WithNumber("depth", mcp.Description("How many hops to traverse (default 1)")),
		mcp.WithString("direction", mcp.Description("Link direction: from, to, or both (default both)")),
	)
}

type relatedResult struct {
	Memory *db.Memory      `json:"memory"`
	Links  []*db.MemoryLink `json:"links"`
}

// Handle processes a get_related tool call.
func (g *GetRelated) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memoryID, err := request.RequireString("memory_id")
	if err != nil {
		return mcp.NewToolResultError("memory_id is required"), nil
	}

	depth := request.GetInt("depth", 1)
	direction := request.GetString("direction", "both")

	if depth <= 0 {
		depth = 1
	}

	// For depth=1, use direct link query with direction filtering
	// For depth>1, use BFS traversal (always bidirectional)
	var memoryIDs []string
	var allLinks []*db.MemoryLink

	if depth == 1 {
		links, err := g.DB.GetLinks(ctx, memoryID, direction)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("getting links: %v", err)), nil
		}
		allLinks = links
		seen := map[string]bool{}
		for _, l := range links {
			neighborID := l.ToID
			if neighborID == memoryID {
				neighborID = l.FromID
			}
			if !seen[neighborID] {
				seen[neighborID] = true
				memoryIDs = append(memoryIDs, neighborID)
			}
		}
	} else {
		ids, err := g.DB.TraverseGraph(ctx, memoryID, depth)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("traversing graph: %v", err)), nil
		}
		memoryIDs = ids
		// Fetch all links for traversed nodes
		links, err := g.DB.GetLinks(ctx, memoryID, "both")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("getting links: %v", err)), nil
		}
		allLinks = links
	}

	// Load full memories
	var results []relatedResult
	for _, id := range memoryIDs {
		mem, err := g.DB.GetMemory(id)
		if err != nil {
			continue // skip if memory was deleted
		}
		// Find links involving this memory
		var relevantLinks []*db.MemoryLink
		for _, l := range allLinks {
			if l.FromID == id || l.ToID == id {
				relevantLinks = append(relevantLinks, l)
			}
		}
		results = append(results, relatedResult{Memory: mem, Links: relevantLinks})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// UnlinkMemories removes a link between memories.
type UnlinkMemories struct {
	DB *db.Client
}

// Tool returns the MCP tool definition for unlink_memories.
func (u *UnlinkMemories) Tool() mcp.Tool {
	return mcp.NewTool("unlink_memories",
		mcp.WithDescription("Remove a link between two memories."),
		mcp.WithString("link_id", mcp.Required(), mcp.Description("Link ID to remove")),
	)
}

// Handle processes an unlink_memories tool call.
func (u *UnlinkMemories) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	linkID, err := request.RequireString("link_id")
	if err != nil {
		return mcp.NewToolResultError("link_id is required"), nil
	}

	if err := u.DB.DeleteLink(ctx, linkID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("deleting link: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Removed link %s", linkID)), nil
}
