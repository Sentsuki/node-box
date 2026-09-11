package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// State is what node-box remembers between runs. It is a cache, not a source of
// truth: deleting it only costs one redundant rewrite of the outputs.
type State struct {
	// Ref is the snapshot that produced the current outputs.
	Ref string `json:"ref"`
	// UpdatedAt is when the last successful run finished.
	UpdatedAt time.Time `json:"updated_at,omitzero"`
	// Outputs maps absolute output path to content hash.
	Outputs map[string]string `json:"outputs,omitempty"`
	// LastError is the failure from the most recent run, empty if it succeeded.
	LastError string `json:"last_error,omitempty"`
	// LastErrorAt is when LastError was recorded.
	LastErrorAt time.Time `json:"last_error_at,omitzero"`
}

// LoadState reads state from path. A missing file yields an empty state, which
// is the correct starting point for a fresh install.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &State{Outputs: map[string]string{}}, nil
		}
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// A corrupt state file must not wedge the process: the worst case of
		// starting over is rewriting outputs that had not changed.
		return &State{Outputs: map[string]string{}}, nil
	}
	if s.Outputs == nil {
		s.Outputs = map[string]string{}
	}
	return &s, nil
}

// Save persists state atomically.
func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	return WriteAtomic(path, data, Perm)
}

// RecordSuccess updates the state after a successful run.
func (s *State) RecordSuccess(ref string, hashes map[string]string) {
	s.Ref = ref
	s.Outputs = hashes
	s.UpdatedAt = time.Now()
	s.LastError = ""
	s.LastErrorAt = time.Time{}
}

// RecordFailure records why the most recent run failed, leaving the output
// hashes from the last success in place.
func (s *State) RecordFailure(err error) {
	if err == nil {
		return
	}
	s.LastError = err.Error()
	s.LastErrorAt = time.Now()
}
