// Package tools implements MCP tool handlers for the magi server.
package tools

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/classify"
	"github.com/j33pguy/magi/contradiction"
	"github.com/j33pguy/magi/db"
	"github.com/j33pguy/magi/embeddings"
)

// Remember stores a new memory with auto-generated embedding.
type Remember struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for remember.
func (r *Remember) Tool() mcp.Tool {
	return mcp.NewTool("remember",
		mcp.WithDescription("Store a memory with automatic semantic embedding. Use this to save information that should be recalled in future conversations."),
		mcp.WithString("content", mcp.Required(), mcp.Description("The content to remember")),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name (e.g. 'iac', 'famtask', 'global')")),
		mcp.WithString("type",
			mcp.Description("Memory type"),
			mcp.Enum("memory", "incident", "lesson", "decision", "project_context", "conversation", "audit", "runbook", "preference", "context", "security", "state"),
		),
		mcp.WithString("summary", mcp.Description("Brief one-line summary of the memory")),
		mcp.WithArray("tags", mcp.Description("Tags for categorization"), mcp.WithStringItems()),
		mcp.WithNumber("dedup_threshold", mcp.Description("Similarity threshold for deduplication (0.0-1.0, default 0.95). Memories above this similarity are considered duplicates.")),
		mcp.WithString("speaker",
			mcp.Description("Who said/wrote this. Default: gilfoyle"),
			mcp.Enum("j33p", "gilfoyle", "agent", "system"),
		),
		mcp.WithString("area",
			mcp.Description("Top-level life/work domain"),
			mcp.Enum("work", "home", "family", "homelab", "project", "meta"),
		),
		mcp.WithString("sub_area",
			mcp.Description("Sub-domain within area (e.g. unifi-vlans, unifi-switches, pihole-ha, vault-cluster). For type=state: use a specific component name so current state is queryable by sub_area. Examples — work: power-platform, fabric, power-bi, sharepoint, teams, azure, td-synnex; homelab: proxmox, networking, security, dns, monitoring, storage, iac, vault, traefik, authentik, lancache; project: magi, distify, labctl, vault-unsealer, iac; home: lego, gaming, streaming, media; family: kids, spouse, schedule"),
		),
	)
}

// Handle processes a remember tool call.
func (r *Remember) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := request.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}

	project, err := request.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil
	}

	memType := request.GetString("type", "memory")
	summary := request.GetString("summary", "")
	tags := request.GetStringSlice("tags", nil)
	speaker := request.GetString("speaker", "gilfoyle")
	area := request.GetString("area", "")
	subArea := request.GetString("sub_area", "")

	// Auto-classify if not explicitly set
	if area == "" || subArea == "" {
		c := classify.Infer(content)
		if area == "" {
			area = c.Area
		}
		if subArea == "" {
			subArea = c.SubArea
		}
	}

	// Check for potential secrets
	if warning := detectSecrets(content); warning != "" {
		return mcp.NewToolResultError(fmt.Sprintf("Content may contain secrets: %s. Remove sensitive data before storing.", warning)), nil
	}

	// Generate embedding
	embedding, err := r.Embedder.Embed(ctx, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating embedding: %v", err)), nil
	}

	// Deduplication: check for near-duplicate before inserting
	dedupThreshold := request.GetFloat("dedup_threshold", 0.95)
	if dedupThreshold < 0 || dedupThreshold > 1 {
		dedupThreshold = 0.95
	}
	// Convert similarity threshold to cosine distance (distance = 1 - similarity)
	maxDistance := 1.0 - dedupThreshold
	// groupDistance is the distance below which we link as parent (similarity 0.85-threshold)
	groupDistance := 0.15 // 1.0 - 0.85

	match, err := r.DB.FindSimilar(embedding, groupDistance)
	if err != nil {
		slog.Warn("dedup check failed, proceeding with insert", "error", err)
	} else if match != nil && match.Distance <= maxDistance {
		// Near-duplicate: return existing memory instead of inserting
		slog.Info("deduplicated memory", "existing_id", match.Memory.ID, "distance", match.Distance)
		return mcp.NewToolResultText(fmt.Sprintf(
			"Deduplicated: existing memory %s is %.1f%% similar (project=%s, type=%s). No new memory created.",
			match.Memory.ID, (1.0-match.Distance)*100, match.Memory.Project, match.Memory.Type,
		)), nil
	}

	// Estimate token count (rough: ~4 chars per token)
	tokenCount := len(content) / 4

	memory := &db.Memory{
		Content:    content,
		Summary:    summary,
		Embedding:  embedding,
		Project:    project,
		Type:       memType,
		Source:     "mcp",
		Speaker:    speaker,
		Area:       area,
		SubArea:    subArea,
		TokenCount: tokenCount,
	}

	// Soft-group: link to similar existing memory as parent
	if match != nil {
		memory.ParentID = match.Memory.ID
		slog.Info("linking memory to similar parent", "parent_id", match.Memory.ID, "distance", match.Distance)
	}

	saved, err := r.DB.SaveMemory(memory)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving memory: %v", err)), nil
	}

	// Append structured taxonomy tags
	if speaker != "" {
		tags = append(tags, "speaker:"+speaker)
	}
	if area != "" {
		tags = append(tags, "area:"+area)
	}
	if subArea != "" {
		tags = append(tags, "sub_area:"+subArea)
	}

	// Set tags
	if len(tags) > 0 {
		if err := r.DB.SetTags(saved.ID, tags); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("setting tags: %v", err)), nil
		}
	}

	msg := fmt.Sprintf("Stored memory %s (project=%s, type=%s, tokens=%d)", saved.ID, project, memType, tokenCount)
	if memory.ParentID != "" {
		msg += fmt.Sprintf(" [linked to similar memory %s, %.1f%% similar]", memory.ParentID, (1.0-match.Distance)*100)
	}

	// Contradiction detection (best-effort, never blocks writes)
	detector := &contradiction.Detector{Threshold: 0.85}
	candidates, cErr := detector.Check(ctx, r.DB, r.Embedder, content, area, subArea)
	if cErr != nil {
		slog.Warn("contradiction detection failed", "error", cErr)
	} else if len(candidates) > 0 {
		msg += fmt.Sprintf("\n\n⚠ %d potential contradiction(s) detected:", len(candidates))
		for _, c := range candidates {
			msg += fmt.Sprintf("\n  - Memory %s (%.0f%% similar, score=%.2f): %s [%s]",
				c.ExistingID, c.Similarity*100, c.Score, c.ExistingSummary, c.Reason)
		}
		msg += "\n\nReview these and consider updating/superseding the old memory if the new one replaces it."
	}

	return mcp.NewToolResultText(msg), nil
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|auth[_-]?token|secret[_-]?key)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-._~+/]+=*`),
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),
	regexp.MustCompile(`-----BEGIN (RSA |EC )?PRIVATE KEY-----`),
}

func detectSecrets(content string) string {
	var found []string
	for _, pat := range secretPatterns {
		if pat.MatchString(content) {
			found = append(found, pat.String())
		}
	}
	if len(found) > 0 {
		return strings.Join(found, "; ")
	}
	return ""
}
