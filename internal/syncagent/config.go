package syncagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root magi-sync configuration.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Machine MachineConfig `yaml:"machine"`
	Sync    SyncConfig    `yaml:"sync"`
	Privacy PrivacyConfig `yaml:"privacy"`
	Agents  []AgentConfig `yaml:"agents"`
}

type ServerConfig struct {
	URL            string `yaml:"url"`
	Token          string `yaml:"token,omitempty"`
	TokenEnv       string `yaml:"token_env,omitempty"`
	EnrollToken    string `yaml:"enroll_token,omitempty"`
	EnrollTokenEnv string `yaml:"enroll_token_env,omitempty"`
	Protocol       string `yaml:"protocol"`
}

type MachineConfig struct {
	ID     string   `yaml:"id"`
	User   string   `yaml:"user"`
	Groups []string `yaml:"groups,omitempty"`
}

// ConflictStrategy determines how to resolve conflicting changes.
type ConflictStrategy string

const (
	ConflictLastWriteWins ConflictStrategy = "last-write-wins"
	ConflictNewest        ConflictStrategy = "newest"
	ConflictManual        ConflictStrategy = "manual"
)

type SyncConfig struct {
	Mode             string           `yaml:"mode"`
	Watch            bool             `yaml:"watch"`
	Interval         string           `yaml:"interval"`
	RetryBackoff     string           `yaml:"retry_backoff"`
	MaxBatchSize     int              `yaml:"max_batch_size"`
	StateFile        string           `yaml:"state_file"`
	ConflictStrategy ConflictStrategy `yaml:"conflict_strategy"`
}

type PrivacyConfig struct {
	Mode          string `yaml:"mode"`
	RedactSecrets bool   `yaml:"redact_secrets"`
	MaxFileSizeKB int64  `yaml:"max_file_size_kb"`
}

type AgentConfig struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`
	Enabled      bool     `yaml:"enabled"`
	Paths        []string `yaml:"paths"`
	Include      []string `yaml:"include"`
	Exclude      []string `yaml:"exclude"`
	Visibility   string   `yaml:"visibility"`
	Owner        string   `yaml:"owner"`
	Viewers      []string `yaml:"viewers"`
	ViewerGroups []string `yaml:"viewer_groups"`
	SettingsPath string   `yaml:"settings_path,omitempty"`
}

// DefaultConfigPath returns the default config location.
func DefaultConfigPath() string {
	return "~/.config/magi-sync/config.yaml"
}

// LoadConfigPath expands the config path.
func LoadConfigPath(path string) (string, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	return expandPath(path)
}

// LoadConfig reads and validates config from disk.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}
	if err := cfg.setDefaults(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) setDefaults() error {
	if c.Server.Protocol == "" {
		c.Server.Protocol = "http"
	}
	if c.Server.Token == "" && c.Server.TokenEnv != "" {
		c.Server.Token = os.Getenv(c.Server.TokenEnv)
	}
	if c.Server.EnrollToken == "" && c.Server.EnrollTokenEnv != "" {
		c.Server.EnrollToken = os.Getenv(c.Server.EnrollTokenEnv)
	}
	if c.Machine.ID == "" {
		host, _ := os.Hostname()
		c.Machine.ID = host
	}
	if c.Machine.User == "" {
		c.Machine.User = os.Getenv("USER")
	}
	if c.Sync.Mode == "" {
		c.Sync.Mode = "push"
	}
	if c.Sync.Interval == "" {
		c.Sync.Interval = "30s"
	}
	if c.Sync.RetryBackoff == "" {
		c.Sync.RetryBackoff = "5s"
	}
	if c.Sync.MaxBatchSize <= 0 {
		c.Sync.MaxBatchSize = 50
	}
	if c.Privacy.Mode == "" {
		c.Privacy.Mode = "allowlist"
	}
	if c.Privacy.MaxFileSizeKB <= 0 {
		c.Privacy.MaxFileSizeKB = 512
	}
	if c.Sync.ConflictStrategy == "" {
		c.Sync.ConflictStrategy = ConflictLastWriteWins
	}
	if c.Sync.StateFile == "" {
		c.Sync.StateFile = "~/.config/magi-sync/state.json"
	}

	var err error
	c.Sync.StateFile, err = expandPath(c.Sync.StateFile)
	if err != nil {
		return fmt.Errorf("expanding state_file: %w", err)
	}
	for i := range c.Agents {
		for j := range c.Agents[i].Paths {
			c.Agents[i].Paths[j], err = expandPath(c.Agents[i].Paths[j])
			if err != nil {
				return fmt.Errorf("expanding agent path: %w", err)
			}
		}
		if c.Agents[i].Visibility == "" {
			c.Agents[i].Visibility = "internal"
		}
		if c.Agents[i].Name == "" {
			c.Agents[i].Name = c.Agents[i].Type
		}
		if c.Agents[i].Owner == "" {
			c.Agents[i].Owner = c.Machine.User
		}
		if c.Agents[i].SettingsPath != "" {
			c.Agents[i].SettingsPath, err = expandPath(c.Agents[i].SettingsPath)
			if err != nil {
				return fmt.Errorf("expanding settings_path: %w", err)
			}
		}
	}
	return nil
}

func (c *Config) validate() error {
	if c.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}
	switch c.Sync.Mode {
	case "push":
	default:
		return fmt.Errorf("unsupported sync.mode %q (phase 1 supports only push)", c.Sync.Mode)
	}
	switch c.Sync.ConflictStrategy {
	case ConflictLastWriteWins, ConflictNewest, ConflictManual:
	default:
		return fmt.Errorf("unsupported conflict_strategy %q", c.Sync.ConflictStrategy)
	}
	switch c.Privacy.Mode {
	case "allowlist", "mixed", "denylist":
	default:
		return fmt.Errorf("unsupported privacy.mode %q", c.Privacy.Mode)
	}
	validVisibility := map[string]bool{
		"private":  true,
		"internal": true,
		"team":     true,
		"shared":   true,
		"public":   true,
	}
	for _, agent := range c.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Type == "" {
			return fmt.Errorf("enabled agent missing type")
		}
		if len(agent.Paths) == 0 {
			return fmt.Errorf("enabled agent %q missing paths", agent.Type)
		}
		if !validVisibility[agent.Visibility] {
			return fmt.Errorf("unsupported visibility %q for agent %q", agent.Visibility, agent.Type)
		}
		if agent.Type == "settings" && agent.SettingsPath == "" {
			return fmt.Errorf("settings agent %q requires settings_path", agent.Name)
		}
	}
	return nil
}

func expandPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		switch p {
		case "~":
			p = home
		default:
			p = filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return filepath.Clean(p), nil
}

// SaveConfig writes the current config back to disk.
func SaveConfig(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding yaml: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
