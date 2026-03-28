package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
	"github.com/j33pguy/claude-memory/search"
)

// Recall performs hybrid search (BM25 + vector + RRF) over stored memories.
type Recall struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for recall.
func (r *Recall) Tool() mcp.Tool {
	return mcp.NewTool("recall",
		mcp.WithDescription("Search stored memories using hybrid retrieval (BM25 keyword + semantic vector search fused via RRF). Returns the most relevant memories. Use 'projects' to query multiple namespaces at once (e.g. your agent namespace + crew:shared). Set 'min_relevance' to filter out low-quality results (0.0-1.0, higher = stricter). If no results pass the threshold, the query is automatically rewritten and retried once."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("project", mcp.Description("Filter by a single project/namespace (e.g. 'agent:gilfoyle', 'crew:shared')")),
		mcp.WithArray("projects", mcp.Description("Filter by multiple namespaces — results from any match (e.g. ['agent:dinesh','crew:shared'])"), mcp.WithStringItems()),
		mcp.WithString("type",
			mcp.Description("Filter by memory type"),
			mcp.Enum("memory", "incident", "lesson", "decision", "project_context", "conversation", "audit", "runbook", "preference", "context", "security"),
		),
		mcp.WithArray("tags", mcp.Description("Filter by tags (any match)"), mcp.WithStringItems()),
		mcp.WithNumber("top_k", mcp.Description("Number of results to return (default 5)")),
		mcp.WithNumber("min_relevance", mcp.Description("Minimum relevance score 0.0-1.0 (default 0.0 = no filtering). Results with score below this are excluded. Score = 1.0 - cosine_distance.")),
		mcp.WithNumber("recency_decay", mcp.Description("Exponential decay rate for recency weighting (default 0.0 = disabled). Recommended: 0.01 (half-life ~70 days). Higher values penalize older memories more.")),
		mcp.WithString("speaker",
			mcp.Description("Filter by speaker: j33p, gilfoyle, agent, system"),
			mcp.Enum("j33p", "gilfoyle", "agent", "system"),
		),
		mcp.WithString("area",
			mcp.Description("Filter by area: work, home, family, homelab, project, meta"),
			mcp.Enum("work", "home", "family", "homelab", "project", "meta"),
		),
		mcp.WithString("sub_area", mcp.Description("Filter by sub-area (e.g. power-platform, proxmox, claude-memory)")),
	)
}

// Handle processes a recall tool call.
func (r *Recall) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	project := request.GetString("project", "")
	projects := request.GetStringSlice("projects", nil)
	memType := request.GetString("type", "")
	tags := request.GetStringSlice("tags", nil)
	topK := request.GetInt("top_k", 5)
	minRelevance := request.GetFloat("min_relevance", 0.0)
	recencyDecay := request.GetFloat("recency_decay", 0.0)
	speaker := request.GetString("speaker", "")
	area := request.GetString("area", "")
	subArea := request.GetString("sub_area", "")

	filter := &db.MemoryFilter{
		Project:    project,
		Projects:   projects,
		Type:       memType,
		Tags:       tags,
		Visibility: "all", // MCP callers (Claude Code, Gilfoyle) see all including private
		Speaker:    speaker,
		Area:       area,
		SubArea:    subArea,
	}

	resp, err := search.Adaptive(ctx, r.DB, r.Embedder.Embed, query, filter, topK, minRelevance, recencyDecay)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search: %v", err)), nil
	}

	if len(resp.Results) == 0 {
		msg := "No matching memories found."
		if resp.Rewritten {
			msg += fmt.Sprintf(" (also tried rewritten query: %q)", resp.RewrittenQuery)
		}
		return mcp.NewToolResultText(msg), nil
	}

	output, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
