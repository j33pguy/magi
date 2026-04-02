package syncagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type Mode string

const (
	ModeEnroll Mode = "enroll"
	ModeCheck  Mode = "check"
	ModeDryRun Mode = "dry-run"
	ModeOnce   Mode = "once"
	ModeRun    Mode = "run"
)

type App struct {
	cfg        *Config
	configPath string
	state      *State
	client     *Client
	logger     *slog.Logger
}

func New(cfg *Config, configPath string, logger *slog.Logger) (*App, error) {
	state, err := LoadState(cfg.Sync.StateFile)
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:        cfg,
		configPath: configPath,
		state:      state,
		client:     NewClient(cfg.Server),
		logger:     logger,
	}, nil
}

func (a *App) Run(ctx context.Context, mode Mode) error {
	if a.cfg.Sync.Mode != "" && a.cfg.Sync.Mode != "push" {
		return fmt.Errorf("unsupported sync.mode %q (phase 1 supports only push)", a.cfg.Sync.Mode)
	}
	switch mode {
	case ModeEnroll:
		return a.enroll(ctx)
	case ModeCheck:
		return a.check(ctx)
	case ModeDryRun:
		return a.sync(ctx, false, true)
	case ModeOnce:
		return a.sync(ctx, true, false)
	case ModeRun:
		return a.loop(ctx)
	default:
		return fmt.Errorf("unsupported mode %q", mode)
	}
}

func (a *App) enroll(ctx context.Context) error {
	resp, err := a.client.Enroll(ctx, a.cfg)
	if err != nil {
		return err
	}
	if resp.Token == "" {
		return fmt.Errorf("enroll succeeded but no machine token was returned")
	}

	a.cfg.Server.Token = resp.Token
	a.cfg.Server.EnrollToken = ""
	a.client.SetToken(resp.Token)
	if err := SaveConfig(a.configPath, a.cfg); err != nil {
		return err
	}

	a.logger.Info("machine enrollment complete",
		"machine", resp.Record.MachineID,
		"user", resp.Record.User,
		"credential_id", resp.Record.ID,
		"config", filepath.Clean(a.configPath),
	)
	return nil
}

func (a *App) check(ctx context.Context) error {
	if err := a.client.Health(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	for _, agent := range a.cfg.Agents {
		if !agent.Enabled {
			continue
		}
		for _, p := range agent.Paths {
			if _, err := os.Stat(p); err != nil {
				a.logger.Warn("configured path is unavailable", "agent", agent.Type, "path", p, "error", err)
				continue
			}
			a.logger.Info("configured path ok", "agent", agent.Type, "path", p)
		}
	}
	count, err := a.scan()
	if err != nil {
		return err
	}
	a.logger.Info("magi-sync check passed", "candidates", count, "server", a.cfg.Server.URL, "state_file", filepath.Clean(a.cfg.Sync.StateFile))
	return nil
}

func (a *App) loop(ctx context.Context) error {
	if err := a.sync(ctx, true, false); err != nil {
		a.logger.Warn("initial sync failed", "error", err)
	}
	d, err := time.ParseDuration(a.cfg.Sync.Interval)
	if err != nil {
		return fmt.Errorf("parse sync interval: %w", err)
	}
	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.sync(ctx, true, false); err != nil {
				a.logger.Warn("sync cycle failed", "error", err)
			}
		}
	}
}

func (a *App) scan() (int, error) {
	payloads, err := a.collectPayloads()
	if err != nil {
		return 0, err
	}
	return len(payloads), nil
}

func (a *App) sync(ctx context.Context, upload bool, dryRun bool) error {
	payloads, err := a.collectPayloads()
	if err != nil {
		return err
	}
	uploaded := 0
	for _, p := range payloads {
		a.logger.Info("candidate payload", "type", p.Type, "project", p.Project, "path", p.SourcePath)
		if dryRun {
			continue
		}
		if upload {
			if err := a.client.Remember(ctx, p); err != nil {
				a.logger.Warn("upload failed", "path", p.SourcePath, "error", err)
				continue
			}
			a.state.Records[checkpointKey(p)] = FileState{SHA256: p.Hash}
			uploaded++
		}
	}
	if upload {
		if err := a.state.Save(a.cfg.Sync.StateFile); err != nil {
			return err
		}
	}
	a.logger.Info("sync complete", "uploaded", uploaded, "dry_run", dryRun)
	return nil
}

func (a *App) collectPayloads() ([]Payload, error) {
	var out []Payload
	for _, agent := range a.cfg.Agents {
		if !agent.Enabled {
			continue
		}
		switch agent.Type {
		case "claude":
			payloads, err := (claudeAdapter{}).Scan(a.cfg, agent, a.cfg.Privacy)
			if err != nil {
				return nil, err
			}
			for _, p := range payloads {
				if prev, ok := a.state.Records[checkpointKey(p)]; ok && prev.SHA256 == p.Hash {
					continue
				}
				out = append(out, p)
			}
		default:
			a.logger.Warn("unsupported agent type in phase 1", "agent", agent.Type)
		}
	}
	return out, nil
}

func NewLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
