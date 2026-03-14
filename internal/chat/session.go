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

// Manager persists and retrieves chat sessions on the local filesystem.
type Manager struct {
	dir string
}

// NewManager returns a Manager that stores sessions under dir.
// If dir is empty, the OS cache directory is used.
func NewManager(dir string) (*Manager, error) {
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("resolving cache dir: %w", err)
		}
		dir = filepath.Join(cache, "zop", "sessions")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating session dir: %w", err)
	}
	return &Manager{dir: dir}, nil
}

// sessionPath returns the file path for the named session.
func (m *Manager) sessionPath(name string) string {
	return filepath.Join(m.dir, name+".json")
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
