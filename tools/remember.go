// Package tools implements MCP tool handlers for the claude-memory server.
package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/claude-memory/db"
	"github.com/j33pguy/claude-memory/embeddings"
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
			mcp.Enum("memory", "incident", "lesson", "decision", "project_context", "conversation", "audit", "runbook", "preference", "context", "security"),
		),
		mcp.WithString("summary", mcp.Description("Brief one-line summary of the memory")),
		mcp.WithArray("tags", mcp.Description("Tags for categorization"), mcp.WithStringItems()),
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

	// Check for potential secrets
	if warning := detectSecrets(content); warning != "" {
		return mcp.NewToolResultError(fmt.Sprintf("Content may contain secrets: %s. Remove sensitive data before storing.", warning)), nil
	}

	// Generate embedding
	embedding, err := r.Embedder.Embed(ctx, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating embedding: %v", err)), nil
	}

	// Estimate token count (rough: ~4 chars per token)
	tokenCount := len(content) / 4

	memory := &db.Memory{
		Content:    content,
		Summary:    summary,
		Embedding:  embedding,
		Project:    project,
		Type:       memType,
		Source:     "claude-code",
		TokenCount: tokenCount,
	}

	saved, err := r.DB.SaveMemory(memory)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving memory: %v", err)), nil
	}

	// Set tags
	if len(tags) > 0 {
		if err := r.DB.SetTags(saved.ID, tags); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("setting tags: %v", err)), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Stored memory %s (project=%s, type=%s, tokens=%d)", saved.ID, project, memType, tokenCount)), nil
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
