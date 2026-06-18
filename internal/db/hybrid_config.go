package db

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultHybridFetchMultiplier = 5
	minimumHybridFetchK          = 20
)

func hybridFetchK(topK int) int {
	if topK <= 0 {
		topK = 10
	}
	multiplier := defaultHybridFetchMultiplier
	if raw := strings.TrimSpace(os.Getenv("MAGI_HYBRID_FETCH_MULTIPLIER")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			multiplier = n
		}
	}
	fetchK := topK * multiplier
	if fetchK < minimumHybridFetchK {
		fetchK = minimumHybridFetchK
	}
	return fetchK
}
