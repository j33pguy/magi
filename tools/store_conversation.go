package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/j33pguy/magi/db"
	"github.com/j33pguy/magi/embeddings"
)

// StoreConversation stores a conversation summary with auto-embedding and tagging.
type StoreConversation struct {
	DB       *db.Client
	Embedder embeddings.Provider
}

// Tool returns the MCP tool definition for store_conversation.
func (s *StoreConversation) Tool() mcp.Tool {
	return mcp.NewTool("store_conversation",
		mcp.WithDescription("Store a conversation summary for cross-channel history. Auto-embeds for semantic search and tags by channel/topics."),
		mcp.WithString("channel", mcp.Required(), mcp.Description("Channel the conversation happened on (e.g. 'mcp', 'slack', 'discord', 'webchat', 'openclaw')")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Summary of the conversation")),
		mcp.WithString("session_key", mcp.Description("Unique session identifier")),
		mcp.WithString("started_at", mcp.Description("When the conversation started (RFC3339)")),
		mcp.WithString("ended_at", mcp.Description("When the conversation ended (RFC3339)")),
		mcp.WithNumber("turn_count", mcp.Description("Number of turns in the conversation")),
		mcp.WithArray("topics", mcp.Description("Topics discussed"), mcp.WithStringItems()),
		mcp.WithArray("decisions", mcp.Description("Decisions made during the conversation"), mcp.WithStringItems()),
		mcp.WithArray("action_items", mcp.Description("Action items from the conversation"), mcp.WithStringItems()),
	)
}

// Handle processes a store_conversation tool call.
func (s *StoreConversation) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channel, err := request.RequireString("channel")
	if err != nil {
		return mcp.NewToolResultError("channel is required"), nil
	}

	summary, err := request.RequireString("summary")
	if err != nil {
		return mcp.NewToolResultError("summary is required"), nil
	}

	sessionKey := request.GetString("session_key", "")
	startedAt := request.GetString("started_at", "")
	endedAt := request.GetString("ended_at", "")
	turnCount := request.GetInt("turn_count", 0)
	topics := request.GetStringSlice("topics", nil)
	decisions := request.GetStringSlice("decisions", nil)
	actionItems := request.GetStringSlice("action_items", nil)

	content := formatConversation(channel, sessionKey, startedAt, endedAt, turnCount, summary, topics, decisions, actionItems)

	embedding, err := s.Embedder.Embed(ctx, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating embedding: %v", err)), nil
	}

	// Deduplication: check for near-duplicate conversation
	const dedupDistance = 0.05 // similarity > 0.95
	const groupDistance = 0.15 // similarity > 0.85

	match, err := s.DB.FindSimilar(embedding, groupDistance)
	if err != nil {
		slog.Warn("conversation dedup check failed, proceeding with insert", "error", err)
	} else if match != nil && match.Distance <= dedupDistance {
		slog.Info("deduplicated conversation", "existing_id", match.Memory.ID, "distance", match.Distance)
		return mcp.NewToolResultText(fmt.Sprintf(
			"Deduplicated: existing conversation %s is %.1f%% similar (channel=%s). No new memory created.",
			match.Memory.ID, (1.0-match.Distance)*100, channel,
		)), nil
	}

	memory := &db.Memory{
		Content:    content,
		Summary:    summary,
		Embedding:  embedding,
		Type:       "conversation",
		Visibility: "private",
		Source:     channel,
		TokenCount: len(content) / 4,
	}

	// Soft-group: link to similar existing memory as parent
	if match != nil {
		memory.ParentID = match.Memory.ID
	}

	saved, err := s.DB.SaveMemory(memory)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving conversation: %v", err)), nil
	}

	tags := []string{"channel:" + channel, "conversation"}
	for _, topic := range topics {
		tags = append(tags, "topic:"+topic)
	}
	if err := s.DB.SetTags(saved.ID, tags); err != nil {
		// Non-fatal: memory is saved, tags can be retried
		return mcp.NewToolResultText(fmt.Sprintf("Stored conversation %s (channel=%s) — warning: tags may not have been saved: %v", saved.ID, channel, err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Stored conversation %s (channel=%s, topics=%v)", saved.ID, channel, topics)), nil
}

func formatConversation(channel, sessionKey, startedAt, endedAt string, turnCount int, summary string, topics, decisions, actionItems []string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Conversation on %s", channel))
	if sessionKey != "" {
		b.WriteString(fmt.Sprintf(" (session: %s)", sessionKey))
	}
	b.WriteString("\n")

	if startedAt != "" || endedAt != "" {
		b.WriteString(fmt.Sprintf("Time: %s to %s\n", startedAt, endedAt))
	}
	if turnCount > 0 {
		b.WriteString(fmt.Sprintf("Turns: %d\n", turnCount))
	}

	if len(topics) > 0 {
		b.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(topics, ", ")))
	}

	b.WriteString("\n")
	b.WriteString(summary)

	if len(decisions) > 0 {
		b.WriteString("\n\nDecisions:\n")
		for _, d := range decisions {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
	}

	if len(actionItems) > 0 {
		b.WriteString("\nAction Items:\n")
		for _, a := range actionItems {
			b.WriteString(fmt.Sprintf("- %s\n", a))
		}
	}

	return b.String()
}
