package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	tool := &RunCommandTool{}
	r.Register(tool)

	got, ok := r.Get("run_command")
	assert.True(t, ok)
	assert.Equal(t, tool, got)

	_, ok = r.Get("non_existent")
	assert.False(t, ok)

	list := r.List()
	require.Len(t, list, 1)
	assert.Equal(t, "run_command", list[0].Name)
}

func TestRunCommandTool(t *testing.T) {
	tool := &RunCommandTool{}
	assert.Equal(t, "run_command", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())

	ctx := context.Background()
	
	// Success case
	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	output, err := tool.Execute(ctx, string(args))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", output)

	// Error case (non-zero exit)
	args, _ = json.Marshal(map[string]string{"command": "false"})
	output, err = tool.Execute(ctx, string(args))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "executing command")

	// Invalid arguments
	_, err = tool.Execute(ctx, "invalid json")
	assert.Error(t, err)
}
