package node

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mode != ModeEmbedded {
		t.Errorf("mode = %q, want %q", cfg.Mode, ModeEmbedded)
	}
	if cfg.WriterPoolSize != 4 {
		t.Errorf("writer pool = %d, want 4", cfg.WriterPoolSize)
	}
	if cfg.ReaderPoolSize != 8 {
		t.Errorf("reader pool = %d, want 8", cfg.ReaderPoolSize)
	}
	if !cfg.CoordinatorEnabled {
		t.Error("coordinator should be enabled by default")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("MAGI_NODE_MODE", "embedded")
	t.Setenv("MAGI_WRITER_POOL_SIZE", "16")
	t.Setenv("MAGI_READER_POOL_SIZE", "32")
	t.Setenv("MAGI_COORDINATOR_ENABLED", "false")

	cfg := ConfigFromEnv()
	if cfg.Mode != ModeEmbedded {
		t.Errorf("mode = %q, want %q", cfg.Mode, ModeEmbedded)
	}
	if cfg.WriterPoolSize != 16 {
		t.Errorf("writer pool = %d, want 16", cfg.WriterPoolSize)
	}
	if cfg.ReaderPoolSize != 32 {
		t.Errorf("reader pool = %d, want 32", cfg.ReaderPoolSize)
	}
	if cfg.CoordinatorEnabled {
		t.Error("coordinator should be disabled")
	}
}

func TestConfigFromEnvInvalidValues(t *testing.T) {
	t.Setenv("MAGI_WRITER_POOL_SIZE", "not-a-number")
	t.Setenv("MAGI_READER_POOL_SIZE", "-5")

	cfg := ConfigFromEnv()
	// Should fall back to defaults.
	if cfg.WriterPoolSize != 4 {
		t.Errorf("writer pool = %d, want 4 (default)", cfg.WriterPoolSize)
	}
	if cfg.ReaderPoolSize != 8 {
		t.Errorf("reader pool = %d, want 8 (default)", cfg.ReaderPoolSize)
	}
}

func TestConfigFromEnvEmpty(t *testing.T) {
	// Ensure env vars are unset.
	os.Unsetenv("MAGI_NODE_MODE")
	os.Unsetenv("MAGI_WRITER_POOL_SIZE")
	os.Unsetenv("MAGI_READER_POOL_SIZE")
	os.Unsetenv("MAGI_COORDINATOR_ENABLED")

	cfg := ConfigFromEnv()
	def := DefaultConfig()
	if cfg.Mode != def.Mode || cfg.WriterPoolSize != def.WriterPoolSize ||
		cfg.ReaderPoolSize != def.ReaderPoolSize || cfg.CoordinatorEnabled != def.CoordinatorEnabled {
		t.Error("empty env should produce default config")
	}
}
