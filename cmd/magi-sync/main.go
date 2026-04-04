package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/j33pguy/magi/internal/syncagent"
)

func main() {
	configPath := flag.String("config", syncagent.DefaultConfigPath(), "Path to magi-sync config file")
	flag.Parse()

	mode := syncagent.ModeOnce
	if flag.NArg() > 0 {
		mode = syncagent.Mode(flag.Arg(0))
	}

	validModes := map[syncagent.Mode]bool{
		syncagent.ModeEnroll: true,
		syncagent.ModeCheck:  true,
		syncagent.ModeDryRun: true,
		syncagent.ModeOnce:   true,
		syncagent.ModeRun:    true,
		syncagent.ModeWatch:  true,
	}
	if !validModes[mode] {
		fmt.Fprintf(os.Stderr, "unknown mode %q; valid modes: enroll, check, dry-run, once, run, watch\n", mode)
		os.Exit(1)
	}

	cfgPath, err := syncagent.LoadConfigPath(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config path error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := syncagent.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	app, err := syncagent.New(cfg, cfgPath, syncagent.NewLogger())
	if err != nil {
		fmt.Fprintf(os.Stderr, "startup error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, mode); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}
}
