// Package approvals persists user-granted approval exceptions (e.g. shell
// command prefixes the user chose to allow permanently) separately from the
// main configuration file.
package approvals

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the state file within the Equus home directory.
const FileName = "approvals.yaml"

// State is the persisted set of approval exceptions.
type State struct {
	ShellAllowPrefixes []string `yaml:"shell_allow_prefixes"`
}

// Store reads and writes the approval state file.
type Store struct{ path string }

// New returns a store rooted at the given Equus home directory.
func New(equusHome string) *Store {
	if strings.TrimSpace(equusHome) == "" {
		return &Store{}
	}
	return &Store{path: filepath.Join(equusHome, FileName)}
}

// Load reads the state, returning empty state when the file is absent.
func (s *Store) Load() (State, error) {
	if s == nil || s.path == "" {
		return State{}, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

// AddShellPrefix records a command prefix, ignoring duplicates, and persists.
func (s *Store) AddShellPrefix(prefix string) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	state, err := s.Load()
	if err != nil {
		return err
	}
	for _, existing := range state.ShellAllowPrefixes {
		if existing == prefix {
			return nil
		}
	}
	state.ShellAllowPrefixes = append(state.ShellAllowPrefixes, prefix)
	return s.save(state)
}

func (s *Store) save(state State) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
