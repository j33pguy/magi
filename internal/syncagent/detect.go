package syncagent

import (
	"os"
	"path/filepath"
)

type DetectedAgent struct {
	Type     string
	BasePath string
	Found    bool
}

type AgentDefaults struct {
	Include []string
	Exclude []string
}

func DetectAgents() []DetectedAgent {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []DetectedAgent{
		{Type: "claude", BasePath: filepath.Join(home, ".claude")},
		{Type: "openclaw", BasePath: filepath.Join(home, ".openclaw")},
		{Type: "codex", BasePath: filepath.Join(home, ".codex")},
	}
	for i := range candidates {
		if info, err := os.Stat(candidates[i].BasePath); err == nil && info.IsDir() {
			candidates[i].Found = true
		}
	}
	return candidates
}

func DefaultAgentDefaults(agentType string) AgentDefaults {
	switch agentType {
	case "claude":
		return AgentDefaults{
			Include: []string{
				"projects/**/*.jsonl",
				"projects/**/CLAUDE.md",
				"projects/**/memory/*.md",
				"memory/**/*.md",
				"plans/**/*.md",
			},
			Exclude: []string{
				"**/tmp/**",
				"**/cache/**",
				"**/*.bin",
				"**/plugins/**",
			},
		}
	case "openclaw":
		return AgentDefaults{
			Include: []string{
				"workspace/**/*.md",
				"agents/*/sessions/*.jsonl",
			},
			Exclude: []string{
				"**/tmp/**",
				"**/cache/**",
				"**/*.bin",
				"**/browser/**",
				"**/canvas/**",
				"**/completions/**",
				"**/credentials/**",
				"**/delivery-queue/**",
				"**/discord/**",
				"**/flows/**",
				"**/media/**",
				"**/subagents/**",
				"**/tasks/**",
				"**/*.tar.gz",
				"**/*.bak*",
				"**/*.tmp",
			},
		}
	default:
		return AgentDefaults{}
	}
}

func DefaultAgentConfig(agentType string, basePath string, owner string) AgentConfig {
	defaults := DefaultAgentDefaults(agentType)
	return AgentConfig{
		Type:    agentType,
		Enabled: true,
		Owner:   owner,
		Paths:   []string{basePath},
		Include: defaults.Include,
		Exclude: defaults.Exclude,
	}
}
