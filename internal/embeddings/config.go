package embeddings

import (
	"os"
	"strconv"
	"strings"
)

// Config controls embedding-provider initialization.
type Config struct {
	CompressionEnabled bool
	CompressionBits    int
}

// ConfigFromEnv reads embedding settings from environment variables.
func ConfigFromEnv() Config {
	cfg := Config{
		CompressionEnabled: false,
		CompressionBits:    4,
	}

	if v := strings.TrimSpace(strings.ToLower(os.Getenv("MAGI_TURBOQUANT_ENABLED"))); v != "" {
		cfg.CompressionEnabled = v == "true" || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("MAGI_TURBOQUANT_BITS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && (n == 2 || n == 4 || n == 8) {
			cfg.CompressionBits = n
		}
	}

	return cfg
}
