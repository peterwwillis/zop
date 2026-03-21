package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	zopapp "github.com/peterwwillis/zop/internal/app"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/peterwwillis/zop/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerDefaults(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	_, err := config.EnsureConfigFile(configPath)
	require.NoError(t, err)

	controller, err := zopapp.NewController(configPath, "mobile", "")
	require.NoError(t, err)

	assert.NotEmpty(t, controller.ActiveAgent())
	assert.Equal(t, configPath, controller.ConfigPath())
	assert.NotEmpty(t, controller.AgentNames())
	_ = controller.MissingAPIKeyWarning()
}

func TestControllerSetAgent(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	_, err := config.EnsureConfigFile(configPath)
	require.NoError(t, err)

	controller, err := zopapp.NewController(configPath, "mobile", "")
	require.NoError(t, err)

	err = controller.SetAgent("does-not-exist")
	assert.Error(t, err)
}

type MockProvider struct {
	CompleteFunc func(ctx context.Context, req provider.CompletionRequest) (provider.CompletionResponse, error)
	Calls        []provider.CompletionRequest
}

func (m *MockProvider) Complete(ctx context.Context, req provider.CompletionRequest) (provider.CompletionResponse, error) {
	m.Calls = append(m.Calls, req)
	return m.CompleteFunc(ctx, req)
}

func (m *MockProvider) Name() string { return "mock" }

func TestControllerToolCalling(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	confContent := `
[tool_policy]
allow_list = [{ tool = "run_command", regex = "echo tool_success" }]
`
	require.NoError(t, os.WriteFile(configPath, []byte(confContent), 0600))

	controller, err := zopapp.NewController(configPath, "test-session", "")
	require.NoError(t, err)

	callCount := 0
	mockProv := &MockProvider{
		CompleteFunc: func(ctx context.Context, req provider.CompletionRequest) (provider.CompletionResponse, error) {
			callCount++
			if callCount == 1 {
				// First call returns a tool call
				return provider.CompletionResponse{
					ToolCalls: []provider.ToolCall{
						{
							ID:        "call_1",
							Name:      "run_command",
							Arguments: `{"command": "echo tool_success"}`,
						},
					},
				}, nil
			}
			// Second call returns final answer
			return provider.CompletionResponse{
				Content: "The tool said tool_success",
			}, nil
		},
	}
	controller.SetProvider(mockProv)

	ctx := context.Background()
	resp, err := controller.SendPrompt(ctx, "run a tool", nil)
	require.NoError(t, err)
	assert.Equal(t, "The tool said tool_success", resp)
	assert.Equal(t, 2, callCount)

	msgs := controller.Messages()
	// system, user, assistant (tool call), tool (result), assistant (final)
	// Actually controller might not have system prompt if not configured.
	// In NewController it adds system prompt if defined.
	// Let's check how many messages we have.
	assert.True(t, len(msgs) >= 4)
	
	lastMsg := msgs[len(msgs)-1]
	assert.Equal(t, "assistant", lastMsg.Role)
	assert.Equal(t, "The tool said tool_success", lastMsg.Content)

	toolResMsg := msgs[len(msgs)-2]
	assert.Equal(t, "tool", toolResMsg.Role)
	assert.Contains(t, toolResMsg.Content, "tool_success")
}
