package ingest

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// contentHash returns the first 16 hex chars of sha256(trimmed content).
// Compatible with tools.contentHash.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return fmt.Sprintf("%x", h[:8])
}
