// Package vcs provides git-backed versioning for MAGI memories.
package vcs

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds git versioning configuration, loaded from environment variables.
type Config struct {
	Enabled       bool
	Path          string        // MAGI_GIT_PATH — root of the git-managed memories repo
	CommitMode    string        // "immediate" or "batch"
	BatchInterval time.Duration // how often to flush pending changes in batch mode
}

// ConfigFromEnv reads git versioning configuration from environment variables.
func ConfigFromEnv() *Config {
	home, _ := os.UserHomeDir()

	enabled, _ := strconv.ParseBool(os.Getenv("MAGI_GIT_ENABLED"))

	gitPath := os.Getenv("MAGI_GIT_PATH")
	if gitPath == "" {
		gitPath = filepath.Join(home, ".magi", "memories")
	}

	commitMode := os.Getenv("MAGI_GIT_COMMIT_MODE")
	if commitMode == "" {
		commitMode = "immediate"
	}

	batchInterval := 30 * time.Second
	if v := os.Getenv("MAGI_GIT_BATCH_INTERVAL"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			batchInterval = time.Duration(secs) * time.Second
		}
	}

	return &Config{
		Enabled:       enabled,
		Path:          gitPath,
		CommitMode:    commitMode,
		BatchInterval: batchInterval,
	}
}
