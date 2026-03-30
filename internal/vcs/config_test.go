package vcs

import (
	"os"
	"testing"
	"time"
)

func TestConfigFromEnv_Defaults(t *testing.T) {
	// Clear all relevant env vars
	os.Unsetenv("MAGI_GIT_ENABLED")
	os.Unsetenv("MAGI_GIT_PATH")
	os.Unsetenv("MAGI_GIT_COMMIT_MODE")
	os.Unsetenv("MAGI_GIT_BATCH_INTERVAL")

	cfg := ConfigFromEnv()

	if cfg.Enabled {
		t.Error("expected Enabled to be false by default")
	}
	if cfg.CommitMode != "immediate" {
		t.Errorf("CommitMode = %q, want %q", cfg.CommitMode, "immediate")
	}
	if cfg.BatchInterval != 30*time.Second {
		t.Errorf("BatchInterval = %v, want %v", cfg.BatchInterval, 30*time.Second)
	}
	// Path should default to ~/.magi/memories
	if cfg.Path == "" {
		t.Error("Path should not be empty")
	}
}

func TestConfigFromEnv_AllSet(t *testing.T) {
	t.Setenv("MAGI_GIT_ENABLED", "true")
	t.Setenv("MAGI_GIT_PATH", "/tmp/test-magi")
	t.Setenv("MAGI_GIT_COMMIT_MODE", "batch")
	t.Setenv("MAGI_GIT_BATCH_INTERVAL", "60")

	cfg := ConfigFromEnv()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true")
	}
	if cfg.Path != "/tmp/test-magi" {
		t.Errorf("Path = %q, want %q", cfg.Path, "/tmp/test-magi")
	}
	if cfg.CommitMode != "batch" {
		t.Errorf("CommitMode = %q, want %q", cfg.CommitMode, "batch")
	}
	if cfg.BatchInterval != 60*time.Second {
		t.Errorf("BatchInterval = %v, want %v", cfg.BatchInterval, 60*time.Second)
	}
}

func TestConfigFromEnv_InvalidBatchInterval(t *testing.T) {
	t.Setenv("MAGI_GIT_BATCH_INTERVAL", "not-a-number")

	cfg := ConfigFromEnv()

	// Should fall back to default
	if cfg.BatchInterval != 30*time.Second {
		t.Errorf("BatchInterval = %v, want %v (default)", cfg.BatchInterval, 30*time.Second)
	}
}

func TestConfigFromEnv_EnabledFalseExplicit(t *testing.T) {
	t.Setenv("MAGI_GIT_ENABLED", "false")

	cfg := ConfigFromEnv()

	if cfg.Enabled {
		t.Error("expected Enabled to be false when set to 'false'")
	}
}

func TestConfigFromEnv_EnabledInvalidValue(t *testing.T) {
	t.Setenv("MAGI_GIT_ENABLED", "not-a-bool")

	cfg := ConfigFromEnv()

	if cfg.Enabled {
		t.Error("expected Enabled to be false for invalid value")
	}
}
