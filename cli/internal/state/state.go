package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type State struct {
	ScanRoot string                       `json:"scanRoot"`
	Projects map[string]map[string]string `json:"projects"`
	PushedAt string                       `json:"pushedAt,omitempty"`
	Version  int                          `json:"version"`
}

func DefaultPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".sentra", "state.json"), nil
}

func Load(filePath string) (State, bool, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, err
	}

	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, false, err
	}

	if s.Version == 0 {
		s.Version = 1
	}
	if s.Projects == nil {
		s.Projects = map[string]map[string]string{}
	}

	return s, true, nil
}

func Save(filePath string, state State) error {
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Projects == nil {
		state.Projects = map[string]map[string]string{}
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, b, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
