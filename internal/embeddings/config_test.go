package embeddings

import "testing"

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("MAGI_TURBOQUANT_ENABLED", "")
	t.Setenv("MAGI_TURBOQUANT_BITS", "")

	cfg := ConfigFromEnv()
	if cfg.CompressionEnabled {
		t.Fatal("CompressionEnabled = true, want false")
	}
	if cfg.CompressionBits != 4 {
		t.Fatalf("CompressionBits = %d, want 4", cfg.CompressionBits)
	}
}

func TestConfigFromEnvOverride(t *testing.T) {
	t.Setenv("MAGI_TURBOQUANT_ENABLED", "1")
	t.Setenv("MAGI_TURBOQUANT_BITS", "2")

	cfg := ConfigFromEnv()
	if !cfg.CompressionEnabled {
		t.Fatal("CompressionEnabled = false, want true")
	}
	if cfg.CompressionBits != 2 {
		t.Fatalf("CompressionBits = %d, want 2", cfg.CompressionBits)
	}
}

func TestConfigFromEnvRejectsInvalidBits(t *testing.T) {
	t.Setenv("MAGI_TURBOQUANT_ENABLED", "true")
	t.Setenv("MAGI_TURBOQUANT_BITS", "3")

	cfg := ConfigFromEnv()
	if !cfg.CompressionEnabled {
		t.Fatal("CompressionEnabled = false, want true")
	}
	if cfg.CompressionBits != 4 {
		t.Fatalf("CompressionBits = %d, want fallback 4", cfg.CompressionBits)
	}
}
