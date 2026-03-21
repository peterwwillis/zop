// Package chat provides conversation session management for zop.
package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/peterwwillis/zop/internal/provider"
)

// validNameRegex is the regexp that session names must satisfy.
var validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,65}$`)

const autoSessionStateFile = ".auto_sessions_state"

type autoSessionState struct {
	LastByAgent map[string]string `json:"last_by_agent"`
}

// Manager persists and retrieves chat sessions on the local filesystem.
type Manager struct {
	dir string
}

// NewManager returns a Manager that stores sessions under dir.
// If dir is empty, the OS cache directory is used.
func NewManager(dir string) (*Manager, error) {
	if dir == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			dir = filepath.Join(cache, "zop", "sessions")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".cache", "zop", "sessions")
		} else {
			dir = "sessions"
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating session dir %q: %w", dir, err)
	}
	return &Manager{dir: dir}, nil
}

// sessionPath returns the file path for the named session.
func (m *Manager) sessionPath(name string) string {
	return filepath.Join(m.dir, name+".json")
}

func (m *Manager) autoStatePath() string {
	return filepath.Join(m.dir, autoSessionStateFile)
}

// ValidateName checks that name conforms to naming rules.
func ValidateName(name string) error {
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("session name %q must be 1-65 chars (a-z, A-Z, 0-9, _, -)", name)
	}
	return nil
}

// Exists reports whether a session with the given name already exists.
func (m *Manager) Exists(name string) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}
	_, err := os.Stat(m.sessionPath(name))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (m *Manager) loadAutoState() (autoSessionState, error) {
	data, err := os.ReadFile(m.autoStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return autoSessionState{LastByAgent: map[string]string{}}, nil
		}
		return autoSessionState{}, fmt.Errorf("reading auto session state: %w", err)
	}

	var state autoSessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return autoSessionState{}, fmt.Errorf("parsing auto session state: %w", err)
	}
	if state.LastByAgent == nil {
		state.LastByAgent = map[string]string{}
	}
	return state, nil
}

func (m *Manager) saveAutoState(state autoSessionState) error {
	if state.LastByAgent == nil {
		state.LastByAgent = map[string]string{}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encoding auto session state: %w", err)
	}
	return os.WriteFile(m.autoStatePath(), data, 0600)
}

// GetLastAutoSession returns the last auto-managed interactive session name for
// the provided agent.
func (m *Manager) GetLastAutoSession(agent string) (string, error) {
	if agent == "" {
		agent = "default"
	}
	state, err := m.loadAutoState()
	if err != nil {
		return "", err
	}
	return state.LastByAgent[agent], nil
}

// SetLastAutoSession stores the last auto-managed interactive session name for
// the provided agent. Passing an empty name clears the mapping.
func (m *Manager) SetLastAutoSession(agent, name string) error {
	if agent == "" {
		agent = "default"
	}
	if name != "" {
		if err := ValidateName(name); err != nil {
			return err
		}
	}

	state, err := m.loadAutoState()
	if err != nil {
		return err
	}
	if name == "" {
		delete(state.LastByAgent, agent)
	} else {
		state.LastByAgent[agent] = name
	}
	return m.saveAutoState(state)
}

// Get loads the message history for a session.
func (m *Manager) Get(name string) ([]provider.Message, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(m.sessionPath(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading session %q: %w", name, err)
	}

	var msgs []provider.Message
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var msg provider.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			return nil, fmt.Errorf("parsing session %q: %w", name, err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Save writes the full message history for a session (overwrites previous).
func (m *Manager) Save(name string, msgs []provider.Message) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	var sb strings.Builder
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshaling session %q: %w", name, err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return os.WriteFile(m.sessionPath(name), []byte(sb.String()), 0600)
}

// List returns all session names stored in the manager directory.
func (m *Manager) List() ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names, nil
}

// Delete removes a session.
func (m *Manager) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	err := os.Remove(m.sessionPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("session %q does not exist", name)
	}
	return err
}
