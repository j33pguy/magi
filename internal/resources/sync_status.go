package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/syncstate"
	"github.com/mark3labs/mcp-go/mcp"
)

// SyncStatus provides current sync status for project-scoped memory.
type SyncStatus struct {
	DB      db.Store
	Project string
	Tracker *syncstate.Tracker
}

// Resource returns the MCP resource definition for sync-status.
func (s *SyncStatus) Resource() mcp.Resource {
	return mcp.NewResource(
		"memory://sync-status",
		"Sync Status",
		mcp.WithResourceDescription("Current sync status for project-scoped memory"),
		mcp.WithMIMEType("application/json"),
	)
}

// Handle returns the current sync status.
func (s *SyncStatus) Handle(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if s.Tracker == nil {
		return nil, fmt.Errorf("sync tracker is not configured")
	}

	snap := s.Tracker.Snapshot()
	maxAge := syncstate.MaxAge()

	lastSync := ""
	secondsAgo := 0
	if !snap.LastSync.IsZero() {
		lastSync = snap.LastSync.UTC().Format(time.RFC3339)
		secondsAgo = int(time.Since(snap.LastSync).Seconds())
	}

	status := "fresh"
	if snap.Syncing {
		status = "syncing"
	} else if snap.LastErr != "" {
		status = "error"
	} else if snap.LastSync.IsZero() || time.Since(snap.LastSync) > maxAge {
		status = "stale"
	}

	count, err := s.DB.CountMemories(&db.MemoryFilter{Project: s.Project, Visibility: "all"})
	if err != nil {
		return nil, fmt.Errorf("counting memories: %w", err)
	}

	payload := struct {
		Project       string `json:"project"`
		LastSync      string `json:"last_sync"`
		SecondsAgo    int    `json:"seconds_ago"`
		PendingWrites int    `json:"pending_writes"`
		MemoryCount   int    `json:"memory_count"`
		Status        string `json:"status"`
	}{
		Project:       s.Project,
		LastSync:      lastSync,
		SecondsAgo:    secondsAgo,
		PendingWrites: 0,
		MemoryCount:   count,
		Status:        status,
	}

	data, err := marshalJSON(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling sync status: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "memory://sync-status",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
