// Package tools implements MCP tool handlers for the magi server.
package tools

import (
	"context"
	"fmt"
	"log/slog"

<<<<<<< release/v0.3.0
	"github.com/j33pguy/magi/internal/classify"
	"github.com/j33pguy/magi/internal/contradiction"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
=======
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
>>>>>>> main
	"github.com/j33pguy/magi/internal/remember"
	"github.com/mark3labs/mcp-go/mcp"
)

// Remember stores a new memory with auto-generated embedding.
type Remember struct {
	DB             db.Store
	Embedder       embeddings.Provider
	DefaultProject string
}

// Tool returns the MCP tool definition for remember.
func (r *Remember) Tool() mcp.Tool {
	return mcp.NewTool("remember",
		mcp.WithDescription("Store a memory with automatic semantic embedding. Use this to save information that should be recalled in future conversations."),
		mcp.WithString("content", mcp.Required(), mcp.Description("The content to remember")),
		mcp.WithString("project", mcp.Description("Project name (auto-detected if omitted)")),
		mcp.WithString("type",
			mcp.Description("Memory type"),
			mcp.Enum("memory", "incident", "lesson", "decision", "project_context", "conversation", "audit", "runbook", "preference", "context", "security", "state"),
		),
		mcp.WithString("summary", mcp.Description("Brief one-line summary of the memory")),
		mcp.WithArray("tags", mcp.Description("Tags for categorization"), mcp.WithStringItems()),
		mcp.WithNumber("dedup_threshold", mcp.Description("Similarity threshold for deduplication (0.0-1.0, default 0.95). Memories above this similarity are considered duplicates.")),
		mcp.WithString("speaker",
			mcp.Description("Who said/wrote this. Default: assistant"),
			mcp.Enum("user", "assistant", "agent", "system"),
		),
		mcp.WithString("area",
			mcp.Description("Top-level life/work domain"),
			mcp.Enum("work", "infrastructure", "development", "personal", "project", "meta"),
		),
		mcp.WithString("sub_area",
			mcp.Description("Sub-domain within area (e.g. database, api-gateway, auth, networking). For type=state: use a specific component name so current state is queryable by sub_area. Examples — work: power-platform, sharepoint, azure; infrastructure: networking, security, dns, monitoring, storage, compute, iac, ci-cd; project: magi, my-app; development: api, database, testing; personal: gaming, hobby"),
		),
	)
}

// Handle processes a remember tool call.
func (r *Remember) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := request.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}

	project := request.GetString("project", "")
	if project == "" {
		project = r.DefaultProject
	}
	if project == "" {
		return mcp.NewToolResultError("project is required"), nil
	}

	memType := request.GetString("type", "memory")
	summary := request.GetString("summary", "")
	tags := request.GetStringSlice("tags", nil)
	speaker := request.GetString("speaker", "assistant")
	area := request.GetString("area", "")
	subArea := request.GetString("sub_area", "")
	dedupThreshold := request.GetFloat("dedup_threshold", 0.95)
	input := remember.Input{
		Content: content,
		Summary: summary,
		Project: project,
		Type:    memType,
		Source:  "mcp",
		Speaker: speaker,
		Area:    area,
		SubArea: subArea,
		Tags:    tags,
	}
	result, err := remember.Remember(ctx, r.DB, r.Embedder, input, remember.Options{
		DedupThreshold:         &dedupThreshold,
		ContradictionThreshold: 0.85,
		TagMode:                remember.TagModeFail,
		Logger:                 slog.Default(),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if result.Deduplicated {
		return mcp.NewToolResultText(fmt.Sprintf(
			"Deduplicated: existing memory %s is %.1f%% similar (project=%s, type=%s). No new memory created.",
			result.Match.Memory.ID, (1.0-result.Match.Distance)*100, result.Match.Memory.Project, result.Match.Memory.Type,
		)), nil
	}

	msg := fmt.Sprintf("Stored memory %s (project=%s, type=%s, tokens=%d)", result.Saved.ID, project, result.Saved.Type, result.Saved.TokenCount)
	if result.Saved.ParentID != "" && result.Match != nil {
		msg += fmt.Sprintf(" [linked to similar memory %s, %.1f%% similar]", result.Saved.ParentID, (1.0-result.Match.Distance)*100)
	}

	if len(result.Contradictions) > 0 {
		msg += fmt.Sprintf("\n\n⚠ %d potential contradiction(s) detected:", len(result.Contradictions))
		for _, c := range result.Contradictions {
			msg += fmt.Sprintf("\n  - Memory %s (%.0f%% similar, score=%.2f): %s [%s]",
				c.ExistingID, c.Similarity*100, c.Score, c.ExistingSummary, c.Reason)
		}
		msg += "\n\nReview these and consider updating/superseding the old memory if the new one replaces it."
	}

	return mcp.NewToolResultText(msg), nil
}
