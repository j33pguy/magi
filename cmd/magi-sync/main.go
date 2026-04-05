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
	interactive := flag.Bool("interactive", false, "Run interactive setup wizard")
	flag.BoolVar(interactive, "i", false, "Run interactive setup wizard")
	flag.Parse()

	mode := syncagent.ModeOnce
	if flag.NArg() > 0 {
		mode = syncagent.Mode(flag.Arg(0))
	}

	if *interactive || mode == syncagent.ModeInit {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		if err := syncagent.RunInteractive(ctx, *configPath, os.Stdin, os.Stdout, syncagent.NewLogger()); err != nil {
			fmt.Fprintf(os.Stderr, "interactive setup error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	validModes := map[syncagent.Mode]bool{
		syncagent.ModeInit:   true,
		syncagent.ModeEnroll: true,
		syncagent.ModeCheck:  true,
		syncagent.ModeDryRun: true,
		syncagent.ModeOnce:   true,
		syncagent.ModeRun:    true,
		syncagent.ModeWatch:  true,
	}
	if !validModes[mode] {
		fmt.Fprintf(os.Stderr, "unknown mode %q; valid modes: init, enroll, check, dry-run, once, run, watch\n", mode)
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
