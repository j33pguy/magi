package syncagent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type prompt struct {
	scanner *bufio.Scanner
	out     io.Writer
}

func RunInteractive(ctx context.Context, configPath string, in io.Reader, out io.Writer, logger *slog.Logger) error {
	if logger == nil {
		logger = NewLogger()
	}
	cfgPath, err := LoadConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("config path error: %w", err)
	}

	p := &prompt{scanner: bufio.NewScanner(in), out: out}
	if info, err := os.Stat(cfgPath); err == nil && !info.IsDir() {
		overwrite, err := p.yesNo(fmt.Sprintf("Config already exists at %s. Overwrite", filepath.Clean(cfgPath)), false)
		if err != nil {
			return err
		}
		if !overwrite {
			fmt.Fprintln(out, "Keeping existing config; setup canceled.")
			return nil
		}
	}

	serverURL, err := p.line("MAGI server URL", "http://localhost:8302")
	if err != nil {
		return err
	}
	for {
		if err := validateServer(ctx, serverURL); err == nil {
			break
		} else {
			fmt.Fprintf(out, "Health check failed for %s: %v\n", serverURL, err)
		}
		retry, err := p.yesNo("Try a different server URL", true)
		if err != nil {
			return err
		}
		if !retry {
			return fmt.Errorf("server health check failed")
		}
		serverURL, err = p.line("MAGI server URL", serverURL)
		if err != nil {
			return err
		}
	}

	enrollToken := ""
	machineToken := ""
	hasEnroll, err := p.yesNo("Do you have an enroll token", false)
	if err != nil {
		return err
	}
	if hasEnroll {
		enrollToken, err = p.line("Enter enroll token", "")
		if err != nil {
			return err
		}
	} else {
		hasMachine, err := p.yesNo("Do you have an existing machine token", false)
		if err != nil {
			return err
		}
		if hasMachine {
			machineToken, err = p.line("Enter machine token", "")
			if err != nil {
				return err
			}
		}
	}

	host, _ := os.Hostname()
	if host == "" {
		host = "unknown-host"
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	machineID, err := p.line("Machine ID", host)
	if err != nil {
		return err
	}
	machineUser, err := p.line("Machine user", user)
	if err != nil {
		return err
	}

	var agents []AgentConfig
	detected := DetectAgents()
	if len(detected) == 0 {
		fmt.Fprintln(out, "No supported agent directories detected.")
	} else {
		for _, d := range detected {
			if !d.Found {
				continue
			}
			useAgent, err := p.yesNo(fmt.Sprintf("Sync %s agent at %s", d.Type, d.BasePath), true)
			if err != nil {
				return err
			}
			if useAgent {
				agents = append(agents, DefaultAgentConfig(d.Type, d.BasePath, machineUser))
			}
		}
	}

	privacyMode, err := p.choice("Privacy mode (allowlist/mixed/denylist)", "allowlist", []string{"allowlist", "mixed", "denylist"})
	if err != nil {
		return err
	}
	redactSecrets, err := p.yesNo("Redact secrets before upload", true)
	if err != nil {
		return err
	}

	cfg := &Config{
		Server: ServerConfig{
			URL:         serverURL,
			Token:       strings.TrimSpace(machineToken),
			EnrollToken: strings.TrimSpace(enrollToken),
			Protocol:    "http",
		},
		Machine: MachineConfig{
			ID:   machineID,
			User: machineUser,
		},
		Sync: SyncConfig{
			Mode:             "push",
			Watch:            true,
			Interval:         "30s",
			RetryBackoff:     "5s",
			MaxBatchSize:     50,
			StateFile:        "~/.config/magi-sync/state.json",
			ConflictStrategy: ConflictLastWriteWins,
		},
		Privacy: PrivacyConfig{
			Mode:          privacyMode,
			RedactSecrets: redactSecrets,
			MaxFileSizeKB: 512,
		},
		Agents: agents,
	}

	if err := SaveConfig(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote config to %s\n", filepath.Clean(cfgPath))

	if strings.TrimSpace(enrollToken) != "" {
		runEnroll, err := p.yesNo("Run enrollment now", false)
		if err != nil {
			return err
		}
		if runEnroll {
			if err := runMode(ctx, cfgPath, ModeEnroll, logger); err != nil {
				return err
			}
		}
	}

	runDry, err := p.yesNo("Run a dry-run sync now", false)
	if err != nil {
		return err
	}
	if runDry {
		if err := runMode(ctx, cfgPath, ModeDryRun, logger); err != nil {
			return err
		}
	}

	return nil
}

func validateServer(ctx context.Context, serverURL string) error {
	client := NewClient(ServerConfig{URL: serverURL})
	return client.Health(ctx)
}

func runMode(ctx context.Context, cfgPath string, mode Mode, logger *slog.Logger) error {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	app, err := New(cfg, cfgPath, logger)
	if err != nil {
		return err
	}
	return app.Run(ctx, mode)
}

func (p *prompt) line(label string, def string) (string, error) {
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", label, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", label)
		}
		value, err := p.readLine()
		if err != nil {
			return "", err
		}
		if value == "" && def != "" {
			return def, nil
		}
		if value != "" {
			return value, nil
		}
	}
}

func (p *prompt) yesNo(label string, def bool) (bool, error) {
	defLabel := "y/N"
	if def {
		defLabel = "Y/n"
	}
	for {
		fmt.Fprintf(p.out, "%s [%s]: ", label, defLabel)
		value, err := p.readLine()
		if err != nil {
			return false, err
		}
		if value == "" {
			return def, nil
		}
		switch strings.ToLower(value) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.out, "Please answer y or n.")
		}
	}
}

func (p *prompt) choice(label string, def string, options []string) (string, error) {
	optionsSet := map[string]bool{}
	for _, option := range options {
		optionsSet[strings.ToLower(option)] = true
	}
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", label, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", label)
		}
		value, err := p.readLine()
		if err != nil {
			return "", err
		}
		if value == "" && def != "" {
			return def, nil
		}
		normalized := strings.ToLower(value)
		if optionsSet[normalized] {
			return normalized, nil
		}
		fmt.Fprintf(p.out, "Please choose one of: %s\n", strings.Join(options, ", "))
	}
}

func (p *prompt) readLine() (string, error) {
	if p.scanner.Scan() {
		return strings.TrimSpace(p.scanner.Text()), nil
	}
	if err := p.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}
