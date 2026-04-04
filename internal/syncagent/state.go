package syncagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// State tracks uploaded files across runs.
type State struct {
	Files   map[string]FileState `json:"files,omitempty"`
	Records map[string]FileState `json:"records"`
}

type FileState struct {
	SHA256       string `json:"sha256"`
	LastSyncHash string `json:"last_sync_hash,omitempty"`
}

func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Records: map[string]FileState{}}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	if s.Records == nil {
		s.Records = map[string]FileState{}
	}
	if len(s.Records) == 0 && len(s.Files) > 0 {
		for k, v := range s.Files {
			s.Records[k] = v
		}
	}
	return &s, nil
}

func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	return nil
}
