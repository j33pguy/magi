// Package tracking provides convenience helpers for common dogfooding patterns.
// These are wrappers around the database store for recording decisions,
// conversations, and legacy task tracking memories.
package tracking

import (
	"context"
	"fmt"
	"strings"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// Tracker provides helpers for writing structured tracking memories.
type Tracker struct {
	DB       db.Store
	Embedder embeddings.Provider
}

// TrackTask writes a task state change as a memory.
//
// Deprecated: task progress now has a dedicated task queue and event log.
// Prefer the separate task queue (`/tasks`, `create_task`, `add_task_event`)
// for new orchestrator/worker coordination flows.
func (t *Tracker) TrackTask(ctx context.Context, id, state string, metadata map[string]string) (*db.Memory, error) {
	var parts []string
	parts = append(parts, fmt.Sprintf("Task %s → %s", id, state))
	for k, v := range metadata {
		parts = append(parts, fmt.Sprintf("  %s: %s", k, v))
	}
	content := strings.Join(parts, "\n")

	emb, err := t.Embedder.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embedding task: %w", err)
	}

	m, err := t.DB.SaveMemory(&db.Memory{
		Content:   content,
		Embedding: emb,
		Type:      "task",
		Source:    "tracking",
		Speaker:   "system",
	})
	if err != nil {
		return nil, fmt.Errorf("saving task: %w", err)
	}

	_ = t.DB.SetTags(m.ID, []string{"task", "state:" + state, "task:" + id})
	return m, nil
}

// TrackDecision writes an architectural decision as a memory.
func (t *Tracker) TrackDecision(ctx context.Context, summary, decisionContext string) (*db.Memory, error) {
	content := fmt.Sprintf("Decision: %s\n\nContext: %s", summary, decisionContext)

	emb, err := t.Embedder.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embedding decision: %w", err)
	}

	m, err := t.DB.SaveMemory(&db.Memory{
		Content:   content,
		Summary:   summary,
		Embedding: emb,
		Type:      "decision",
		Source:    "tracking",
		Speaker:   "system",
	})
	if err != nil {
		return nil, fmt.Errorf("saving decision: %w", err)
	}

	_ = t.DB.SetTags(m.ID, []string{"decision", "architectural"})
	return m, nil
}

// TrackConversation writes a conversation summary as a memory.
func (t *Tracker) TrackConversation(ctx context.Context, summary string, topics, decisions, actionItems []string) (*db.Memory, error) {
	var parts []string
	parts = append(parts, "Conversation Summary: "+summary)
	if len(topics) > 0 {
		parts = append(parts, "\nTopics: "+strings.Join(topics, ", "))
	}
	if len(decisions) > 0 {
		parts = append(parts, "\nDecisions:")
		for _, d := range decisions {
			parts = append(parts, "  - "+d)
		}
	}
	if len(actionItems) > 0 {
		parts = append(parts, "\nAction Items:")
		for _, a := range actionItems {
			parts = append(parts, "  - "+a)
		}
	}
	content := strings.Join(parts, "\n")

	emb, err := t.Embedder.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embedding conversation: %w", err)
	}

	m, err := t.DB.SaveMemory(&db.Memory{
		Content:    content,
		Summary:    summary,
		Embedding:  emb,
		Type:       "conversation",
		Source:     "tracking",
		Speaker:    "system",
		Visibility: "internal",
	})
	if err != nil {
		return nil, fmt.Errorf("saving conversation: %w", err)
	}

	tags := []string{"conversation", "tracking"}
	for _, topic := range topics {
		tags = append(tags, "topic:"+topic)
	}
	_ = t.DB.SetTags(m.ID, tags)
	return m, nil
}
