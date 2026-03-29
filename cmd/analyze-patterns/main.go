// Package main provides a CLI tool for analyzing behavioral patterns
// from the magi database.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/patterns"
)

func main() {
	days := flag.Int("days", 90, "Number of days of memories to analyze")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := context.Background()

	// Initialize database
	dbCfg := db.ConfigFromEnv()
	dbClient, err := db.NewStore(dbCfg, logger.WithGroup("db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	defer dbClient.Close()

	if err := dbClient.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "migration error: %v\n", err)
		os.Exit(1)
	}

	// Initialize embedding provider
	embedder, err := embeddings.NewOnnxProvider(logger.WithGroup("embeddings"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "embeddings error: %v\n", err)
		os.Exit(1)
	}
	defer embedder.Destroy()

	// Fetch recent memories from the user
	since := time.Now().AddDate(0, 0, -*days)
	memories, err := dbClient.ListMemories(&db.MemoryFilter{
		Speaker:    "user",
		AfterTime:  &since,
		Limit:      1000,
		Visibility: "all",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing memories: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Fetched memories for analysis", "count", len(memories), "days", *days)

	// Run analyzer
	analyzer := &patterns.Analyzer{}
	detected := analyzer.Analyze(memories)
	logger.Info("Patterns detected", "count", len(detected))

	for _, p := range detected {
		logger.Info("Pattern",
			"type", p.Type,
			"description", p.Description,
			"confidence", fmt.Sprintf("%.0f%%", p.Confidence*100),
			"evidence_count", len(p.Evidence),
		)
	}

	// Store patterns
	stored, skippedDups, err := patterns.StorePatterns(ctx, dbClient, embedder, detected)
	if err != nil {
		fmt.Fprintf(os.Stderr, "storing patterns: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Pattern analysis complete",
		"patterns_found", len(detected),
		"patterns_stored", len(stored),
		"skipped_duplicates", skippedDups,
	)
}
