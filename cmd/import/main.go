// Package main provides a CLI tool for importing existing markdown memory files
// into the magi database.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/j33pguy/magi/internal/chunking"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/migrate"
)

func main() {
	dir := flag.String("dir", "", "Directory containing markdown memory files")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "Usage: magi-import --dir <path>")
		os.Exit(1)
	}

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

	importer := &migrate.MarkdownImporter{
		DB:       dbClient,
		Embedder: embedder,
		Splitter: chunking.NewSplitter(),
		Logger:   logger,
	}

	mappings := migrate.DefaultMappings()

	if err := importer.Import(ctx, *dir, mappings); err != nil {
		fmt.Fprintf(os.Stderr, "import error: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Import complete")
}
