package syncagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Mode string

const (
	ModeEnroll Mode = "enroll"
	ModeCheck  Mode = "check"
	ModeDryRun Mode = "dry-run"
	ModeOnce   Mode = "once"
	ModeRun    Mode = "run"
	ModeWatch  Mode = "watch"
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
	case ModeWatch:
		return a.watch(ctx)
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

func (a *App) watch(ctx context.Context) error {
	// Initial sync on startup.
	if err := a.sync(ctx, true, false); err != nil {
		a.logger.Warn("initial sync failed", "error", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	// Collect directories to watch from enabled agents.
	for _, agent := range a.cfg.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Type == "settings" && agent.SettingsPath != "" {
			dir := filepath.Dir(agent.SettingsPath)
			if err := watcher.Add(dir); err != nil {
				a.logger.Warn("failed to watch settings dir", "path", dir, "error", err)
			}
			continue
		}
		for _, p := range agent.Paths {
			if err := a.watchRecursive(watcher, p); err != nil {
				a.logger.Warn("failed to watch path", "path", p, "error", err)
			}
		}
	}

	a.logger.Info("watch mode started, waiting for file changes")

	const debounce = 500 * time.Millisecond
	timer := time.NewTimer(debounce)
	timer.Stop()
	pending := false

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("watch mode shutting down")
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			if !a.matchesAgent(event.Name) {
				continue
			}
			a.logger.Info("file change detected", "path", event.Name, "op", event.Op.String())
			// Watch new directories created under watched paths.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = a.watchRecursive(watcher, event.Name)
				}
			}
			if !pending {
				timer.Reset(debounce)
				pending = true
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			a.logger.Warn("watcher error", "error", err)
		case <-timer.C:
			pending = false
			if err := a.sync(ctx, true, false); err != nil {
				a.logger.Warn("sync failed", "error", err)
			}
		}
	}
}

// watchRecursive adds a directory and all subdirectories to the watcher.
func (a *App) watchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if info.IsDir() {
			if err := watcher.Add(path); err != nil {
				a.logger.Warn("failed to watch directory", "path", path, "error", err)
				return nil
			}
			a.logger.Info("watching directory", "path", path)
		}
		return nil
	})
}

// matchesAgent checks if a file path matches any enabled agent's patterns.
func (a *App) matchesAgent(path string) bool {
	for _, agent := range a.cfg.Agents {
		if !agent.Enabled {
			continue
		}
		// Settings agent watches its specific settings file.
		if agent.Type == "settings" && agent.SettingsPath != "" {
			if filepath.Clean(path) == filepath.Clean(agent.SettingsPath) {
				return true
			}
			continue
		}
		for _, base := range agent.Paths {
			rel, err := filepath.Rel(base, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			if shouldInclude(rel, agent.Include, agent.Exclude) {
				return true
			}
		}
	}
	return false
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
			a.state.Records[checkpointKey(p)] = FileState{SHA256: p.Hash, LastSyncHash: p.Hash}
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
		var payloads []Payload
		switch agent.Type {
		case "claude":
			var err error
			payloads, err = (claudeAdapter{}).Scan(a.cfg, agent, a.cfg.Privacy)
			if err != nil {
				return nil, err
			}
		case "openclaw":
			var err error
			payloads, err = (openclawAdapter{}).Scan(a.cfg, agent, a.cfg.Privacy)
			if err != nil {
				return nil, err
			}
		case "settings":
			var err error
			payloads, err = (settingsAdapter{}).Scan(a.cfg, agent)
			if err != nil {
				return nil, err
			}
		default:
			a.logger.Warn("unsupported agent type", "agent", agent.Type)
			continue
		}
		for _, p := range payloads {
			if prev, ok := a.state.Records[checkpointKey(p)]; ok && prev.SHA256 == p.Hash {
				continue
			}
			out = append(out, p)
		}
	}
	return out, nil
}

func NewLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
