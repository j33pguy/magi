package syncagent

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SettingsFile represents the on-disk settings structure.
type SettingsFile struct {
	Preferences map[string]any `yaml:"preferences" json:"preferences"`
}

type settingsAdapter struct{}

// Scan reads a settings YAML file and produces a single Payload of type
// "preference" tagged with settings and machine info.
func (settingsAdapter) Scan(cfg *Config, agent AgentConfig) ([]Payload, error) {
	if agent.SettingsPath == "" {
		return nil, fmt.Errorf("settings agent %q: settings_path is required", agent.Name)
	}

	data, err := os.ReadFile(agent.SettingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no settings file yet
		}
		return nil, fmt.Errorf("reading settings %s: %w", agent.SettingsPath, err)
	}

	var sf SettingsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing settings %s: %w", agent.SettingsPath, err)
	}
	if len(sf.Preferences) == 0 {
		return nil, nil
	}

	content, err := json.Marshal(sf.Preferences)
	if err != nil {
		return nil, fmt.Errorf("encoding settings: %w", err)
	}

	tags := settingsTags(cfg, agent)
	p := Payload{
		Content:    string(content),
		Summary:    settingsSummary(sf.Preferences),
		Project:    "magi",
		Type:       "preference",
		Visibility: agent.Visibility,
		Tags:       tags,
		Source:     "magi-sync",
		Speaker:    "system",
		SourcePath: agent.SettingsPath,
		Hash:       hashBytes(data),
	}
	p.Key = checkpointKey(p)
	return []Payload{p}, nil
}

// MergeSettings merges remote preferences into the local settings file.
// For each key, the version with the newer timestamp wins.
func MergeSettings(localPath string, remotePrefs map[string]any, remoteTime time.Time) error {
	var local SettingsFile

	data, err := os.ReadFile(localPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading local settings: %w", err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, &local); err != nil {
			return fmt.Errorf("parsing local settings: %w", err)
		}
	}
	if local.Preferences == nil {
		local.Preferences = make(map[string]any)
	}

	localInfo, _ := os.Stat(localPath)
	localTime := time.Time{}
	if localInfo != nil {
		localTime = localInfo.ModTime()
	}

	// For each remote key, newer timestamp wins.
	for k, v := range remotePrefs {
		if _, exists := local.Preferences[k]; !exists {
			// Key doesn't exist locally — take remote.
			local.Preferences[k] = v
		} else if remoteTime.After(localTime) {
			// Both exist — remote is newer.
			local.Preferences[k] = v
		}
		// Otherwise local is newer or same — keep local.
	}

	out, err := yaml.Marshal(&local)
	if err != nil {
		return fmt.Errorf("encoding merged settings: %w", err)
	}
	return os.WriteFile(localPath, out, 0o644)
}

func settingsTags(cfg *Config, agent AgentConfig) []string {
	tags := []string{
		"source:magi_sync",
		"settings",
		"agent:" + agent.Type,
		"agent_name:" + agent.Name,
		"type:preference",
		"visibility:" + agent.Visibility,
	}
	if cfg != nil {
		if cfg.Machine.ID != "" {
			tags = append(tags, "machine:"+cfg.Machine.ID)
		}
		if cfg.Machine.User != "" {
			tags = append(tags, "user:"+cfg.Machine.User)
		}
		if cfg.Machine.User != "" && cfg.Machine.ID != "" {
			tags = append(tags, "identity:"+cfg.Machine.User+"."+cfg.Machine.ID)
		}
	}
	if agent.Owner != "" {
		tags = append(tags, "owner:"+agent.Owner)
	}
	return dedupeStrings(tags)
}

func settingsSummary(prefs map[string]any) string {
	keys := make([]string, 0, len(prefs))
	for k := range prefs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	summary := "settings: " + strings.Join(keys, ", ")
	if len(summary) > 120 {
		return summary[:120]
	}
	return summary
}
