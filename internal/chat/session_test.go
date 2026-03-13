package chat_test

import (
	"testing"

	"github.com/peterwwillis/pgpt/internal/chat"
	"github.com/peterwwillis/pgpt/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionRoundtrip(t *testing.T) {
	dir := t.TempDir()
	m, err := chat.NewManager(dir)
	require.NoError(t, err)

	msgs := []provider.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	require.NoError(t, m.Save("test-session", msgs))

	loaded, err := m.Get("test-session")
	require.NoError(t, err)
	assert.Equal(t, msgs, loaded)
}

func TestSessionList(t *testing.T) {
	dir := t.TempDir()
	m, err := chat.NewManager(dir)
	require.NoError(t, err)

	require.NoError(t, m.Save("session-a", []provider.Message{{Role: "user", Content: "a"}}))
	require.NoError(t, m.Save("session-b", []provider.Message{{Role: "user", Content: "b"}}))

	names, err := m.List()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"session-a", "session-b"}, names)
}

func TestSessionDelete(t *testing.T) {
	dir := t.TempDir()
	m, err := chat.NewManager(dir)
	require.NoError(t, err)

	require.NoError(t, m.Save("to-delete", []provider.Message{{Role: "user", Content: "bye"}}))

	exists, err := m.Exists("to-delete")
	require.NoError(t, err)
	assert.True(t, exists)

	require.NoError(t, m.Delete("to-delete"))

	exists, err = m.Exists("to-delete")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSessionDeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	m, err := chat.NewManager(dir)
	require.NoError(t, err)
	assert.Error(t, m.Delete("ghost"))
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid-name", false},
		{"valid_name123", false},
		{"a", false},
		{"", true},
		{"has space", true},
		{"has/slash", true},
		{"toolooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong", true},
	}
	for _, tc := range tests {
		err := chat.ValidateName(tc.name)
		if tc.wantErr {
			assert.Error(t, err, "expected error for name %q", tc.name)
		} else {
			assert.NoError(t, err, "expected no error for name %q", tc.name)
		}
	}
}
