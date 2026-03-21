package tool

import (
	"testing"

	"github.com/peterwwillis/zop/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestPolicyChecker(t *testing.T) {
	policy := config.ToolPolicy{
		AllowList: []config.ToolEntry{
			{Exact: []string{"ls", "-la"}},
			{Regex: `^echo\s+.*$`},
			{RegexArray: []string{"cat", `.*\.txt$`}, Tags: []string{"safe"}},
			{Exact: []string{"rm", "-rf", "/tmp/safe"}, Tags: []string{"dangerous"}},
		},
		DenyList: []config.ToolEntry{
			{Exact: []string{"ls", "/"}},
			{Regex: `.*;.*`}, // Deny shell chaining
		},
		DenyTags: []string{"dangerous"},
	}

	pc := NewPolicyChecker(policy)

	// Allowed by Exact
	assert.True(t, pc.IsAllowed("ls -la"))
	// Denied by DenyList Exact
	assert.False(t, pc.IsAllowed("ls /"))
	// Allowed by Regex
	assert.True(t, pc.IsAllowed("echo hello world"))
	// Denied by DenyList Regex (shell chaining)
	assert.False(t, pc.IsAllowed("echo hello; rm -rf /"))
	// Allowed by RegexArray
	assert.True(t, pc.IsAllowed("cat notes.txt"))
	// Denied by RegexArray (wrong extension)
	assert.False(t, pc.IsAllowed("cat script.sh"))
	// Denied by DenyTags
	assert.False(t, pc.IsAllowed("rm -rf /tmp/safe"))
	// Denied because not in AllowList
	assert.False(t, pc.IsAllowed("whoami"))
}

func TestPolicyChecker_RestrictiveByDefault(t *testing.T) {
	// Empty AllowList should deny everything
	policy := config.ToolPolicy{
		DenyList: []config.ToolEntry{
			{Exact: []string{"rm", "-rf", "/"}},
		},
	}
	pc := NewPolicyChecker(policy)

	assert.False(t, pc.IsAllowed("ls -la"))
	assert.False(t, pc.IsAllowed("rm -rf /"))
}

func TestPolicyChecker_Detokenize(t *testing.T) {
	pc := &PolicyChecker{}
	assert.Equal(t, []string{"ls", "-la", "/tmp"}, pc.detokenize("ls -la /tmp"))
	assert.Equal(t, []string{"echo", "hello world"}, pc.detokenize("echo 'hello world'"))
	assert.Equal(t, []string{"echo", "hello world"}, pc.detokenize(`echo "hello world"`))
	assert.Equal(t, []string{"ls", "-l", "file with space.txt"}, pc.detokenize(`ls -l "file with space.txt"`))
}

func TestPolicyChecker_Tags(t *testing.T) {
	policy := config.ToolPolicy{
		AllowList: []config.ToolEntry{
			{Exact: []string{"ls"}, Tags: []string{"fs", "read"}},
			{Exact: []string{"rm"}, Tags: []string{"fs", "write"}},
		},
		AllowTags: []string{"read"},
		DenyTags:  []string{"write"},
	}
	pc := NewPolicyChecker(policy)

	// ls has 'read' tag which is allowed
	assert.True(t, pc.IsAllowed("ls"))
	// rm has 'write' tag which is denied
	assert.False(t, pc.IsAllowed("rm"))
	// whoami is not in allow list
	assert.False(t, pc.IsAllowed("whoami"))
}
