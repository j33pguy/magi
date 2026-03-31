package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/syncstate"
	"github.com/mark3labs/mcp-go/mcp"
)

// SyncNow forces a manual sync with the remote store (if supported).
type SyncNow struct {
	DB      db.Store
	Project string
	Tracker *syncstate.Tracker
}

// Tool returns the MCP tool definition for sync_now.
func (s *SyncNow) Tool() mcp.Tool {
	return mcp.NewTool("sync_now",
		mcp.WithDescription("Force a manual sync of project memories."),
	)
}

// Handle processes a sync_now tool call.
func (s *SyncNow) Handle(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.Tracker == nil {
		return mcp.NewToolResultError("sync tracker is not configured"), nil
	}

	syncer, ok := s.DB.(interface{ Sync() error })
	if !ok {
		return mcp.NewToolResultError("sync is not supported by this backend"), nil
	}

	if started := s.Tracker.Start(); started {
		err := syncer.Sync()
		s.Tracker.Finish(err)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
		}
	} else {
		s.Tracker.Wait()
	}

	count, err := s.DB.CountMemories(&db.MemoryFilter{Project: s.Project, Visibility: "all"})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("counting memories: %v", err)), nil
	}

	snap := s.Tracker.Snapshot()
	syncedAt := ""
	if !snap.LastSync.IsZero() {
		syncedAt = snap.LastSync.UTC().Format("2006-01-02T15:04:05Z")
	}

	payload := struct {
		SyncedAt      string `json:"synced_at"`
		RecordsPulled int    `json:"records_pulled"`
		Project       string `json:"project"`
	}{
		SyncedAt:      syncedAt,
		RecordsPulled: count,
		Project:       s.Project,
	}

	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(out)), nil
}
