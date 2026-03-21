package tool

import (
	"testing"

	"github.com/peterwwillis/zop/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestPolicyChecker(t *testing.T) {
	policy := config.ToolPolicy{
		AllowList: []config.ToolEntry{
			{Tool: "run_command", Exact: []string{"ls", "-la"}},
			{Tool: "run_command", Regex: `^echo\s+.*$`},
			{Tool: "run_command", RegexArray: []string{"cat", `.*\.txt$`}, Tags: []string{"safe"}},
			{Tool: "run_command", Exact: []string{"rm", "-rf", "/tmp/safe"}, Tags: []string{"dangerous"}},
			{Tool: "mcp.list_files", Tags: []string{"safe"}},
			{Tool: "mcp.read_file", Regex: "notes.txt"},
		},
		DenyList: []config.ToolEntry{
			{Tool: "run_command", Exact: []string{"ls", "/"}},
			{Tool: "run_command", Regex: `.*;.*`}, // Deny shell chaining
			{Tool: "mcp.delete_file"},
		},
		DenyTags: []string{"dangerous"},
	}

	pc := NewPolicyChecker(policy)

	// run_command checks
	assert.True(t, pc.IsAllowed("run_command", `{"command": "ls -la"}`))
	assert.False(t, pc.IsAllowed("run_command", `{"command": "ls /"}`))
	assert.True(t, pc.IsAllowed("run_command", `{"command": "echo hello"}`))
	assert.False(t, pc.IsAllowed("run_command", `{"command": "rm -rf /tmp/safe"}`)) // denied by tag

	// MCP checks
	assert.True(t, pc.IsAllowed("mcp.list_files", `{"path": "."}`))
	assert.True(t, pc.IsAllowed("mcp.read_file", `{"path": "notes.txt"}`))
	assert.False(t, pc.IsAllowed("mcp.read_file", `{"path": "secrets.env"}`))
	assert.False(t, pc.IsAllowed("mcp.delete_file", `{"path": "notes.txt"}`))

	// Implicit run_command (Tool="")
	assert.True(t, pc.IsAllowed("run_command", `{"command": "ls -la"}`))
}

func TestPolicyChecker_RestrictiveByDefault(t *testing.T) {
	// Empty AllowList should deny everything
	policy := config.ToolPolicy{
		DenyList: []config.ToolEntry{
			{Tool: "run_command", Exact: []string{"rm", "-rf", "/"}},
		},
	}
	pc := NewPolicyChecker(policy)

	assert.False(t, pc.IsAllowed("run_command", `{"command": "ls"}`))
	assert.False(t, pc.IsAllowed("mcp.list_files", `{}`))
}

func TestPolicyChecker_Detokenize(t *testing.T) {
	pc := &PolicyChecker{}
	assert.Equal(t, []string{"ls", "-la", "/tmp"}, pc.detokenize("ls -la /tmp"))
	assert.Equal(t, []string{"echo", "hello world"}, pc.detokenize("echo 'hello world'"))
}
