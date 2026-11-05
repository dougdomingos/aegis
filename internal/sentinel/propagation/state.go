// Package propagation applies and durably stores the rule state delivered by
// the Orchestrator, and keeps the Sentinel's stream alive across failures.
package propagation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dougdomingos.com/aegis/internal/common/propagation"
)

// State is the durable applied state of one machine: the last accepted
// epoch/sequence, the policy identity behind it, and the rules in force.
type State struct {
	Epoch         string             `json:"epoch"`
	LastSequence  int64              `json:"last_sequence"`
	PolicyID      *int64             `json:"policy_id"`
	PolicyName    string             `json:"policy_name"`
	PolicyVersion int                `json:"policy_version"`
	PolicyType    string             `json:"policy_type"`
	Rules         []propagation.Rule `json:"rules"`
}

// FileStore persists the applied state in a single JSON file, written
// atomically through a temporary file in the same directory.
type FileStore struct {
	path string
}

// NewFileStore builds a FileStore for the provided path, creating its parent
// directory when missing. The path is required.
func NewFileStore(path string) (*FileStore, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("rules state path must not be empty")
	}

	if err := os.MkdirAll(filepath.Dir(trimmed), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create rules state directory: %w", err)
	}

	return &FileStore{path: trimmed}, nil
}

// Load reads the applied state, returning the zero state when the file does
// not exist yet. A malformed file is reported as an error.
func (store *FileStore) Load() (State, error) {
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Rules: []propagation.Rule{}}, nil
		}

		return State{}, fmt.Errorf("failed to read rules state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("failed to parse rules state: %w", err)
	}

	if state.Rules == nil {
		state.Rules = []propagation.Rule{}
	}

	return state, nil
}

// Save atomically replaces the state file: the payload is written to a
// temporary file with mode 0600, synced, and renamed over the target, so a
// crash never leaves a half-written state.
func (store *FileStore) Save(state State) error {
	directory := filepath.Dir(store.path)

	temporary, err := os.CreateTemp(directory, ".rules-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary rules state: %w", err)
	}

	temporaryPath := temporary.Name()

	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !os.IsNotExist(err) {
			// The file was already renamed away or never existed: nothing to clean.
			_ = err
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("failed to set rules state permissions: %w", err)
	}

	if err := json.NewEncoder(temporary).Encode(state); err != nil {
		temporary.Close()
		return fmt.Errorf("failed to encode rules state: %w", err)
	}

	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("failed to sync rules state: %w", err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close rules state: %w", err)
	}

	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("failed to commit rules state: %w", err)
	}

	return nil
}
