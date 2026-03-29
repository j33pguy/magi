package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/internal/classify"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/ingest"
)

// IngestConversation imports a conversation export into memory.
type IngestConversation struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for ingest_conversation.
func (t *IngestConversation) Tool() mcp.Tool {
	return mcp.NewTool("ingest_conversation",
		mcp.WithDescription("Import a conversation export (Grok, ChatGPT, or plain text) into memory. Auto-detects format and extracts decisions, lessons, preferences, and context."),
		mcp.WithString("content", mcp.Required(), mcp.Description("Raw conversation text or JSON export")),
		mcp.WithString("source", mcp.Description("Source format: grok, chatgpt, plaintext, or auto (default: auto)")),
		mcp.WithString("project", mcp.Description("Associate all memories with this project")),
		mcp.WithBoolean("dry_run", mcp.Description("If true, return what would be imported without storing (default: false)")),
	)
}

// Handle processes an ingest_conversation tool call.
func (t *IngestConversation) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := request.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil
	}

	project := request.GetString("project", "")
	dryRun := request.GetBool("dry_run", false)

	data := []byte(content)
	conv, err := ingest.Parse(data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("parse error: %v", err)), nil
	}

	candidates := ingest.ExtractMemories(conv)

	dedup := &ingest.Deduplicator{DB: t.DB, Embedder: t.Embedder}
	kept, skipped, err := dedup.Filter(ctx, candidates)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dedup error: %v", err)), nil
	}

	if dryRun {
		type preview struct {
			Type    string `json:"type"`
			Summary string `json:"summary"`
			Speaker string `json:"speaker"`
		}
		var previews []preview
		for _, em := range kept {
			previews = append(previews, preview{
				Type:    em.Type,
				Summary: em.Summary,
				Speaker: em.Speaker,
			})
		}
		result := struct {
			Format    string    `json:"format"`
			WouldImport int    `json:"would_import"`
			WouldSkip   int    `json:"would_skip"`
			Memories  []preview `json:"memories"`
		}{
			Format:      string(conv.Format),
			WouldImport: len(kept),
			WouldSkip:   skipped,
			Memories:    previews,
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(out)), nil
	}

	var imported int
	var ids []string
	for _, em := range kept {
		c := classify.Infer(em.Content)
		embedding, err := t.Embedder.Embed(ctx, em.Content)
		if err != nil {
			slog.Error("embedding failed during ingest", "error", err)
			continue
		}

		mem := &db.Memory{
			Content:    em.Content,
			Summary:    em.Summary,
			Embedding:  embedding,
			Project:    project,
			Type:       em.Type,
			Source:     em.Source,
			Speaker:    em.Speaker,
			Area:       c.Area,
			SubArea:    c.SubArea,
			TokenCount: len(em.Content) / 4,
		}

		saved, err := t.DB.SaveMemory(mem)
		if err != nil {
			slog.Error("save failed during ingest", "error", err)
			continue
		}

		tags := append(em.Tags, "speaker:"+em.Speaker)
		if c.Area != "" {
			tags = append(tags, "area:"+c.Area)
		}
		if c.SubArea != "" {
			tags = append(tags, "sub_area:"+c.SubArea)
		}
		if err := t.DB.SetTags(saved.ID, tags); err != nil {
			slog.Error("set tags failed during ingest", "error", err)
		}

		imported++
		ids = append(ids, saved.ID)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Ingested %d memories from %s conversation (skipped %d duplicates). IDs: %v",
		imported, conv.Format, skipped, ids,
	)), nil
}
