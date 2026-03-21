package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peterwwillis/zop/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	path := tempConfigPath(t)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Contains(t, cfg.Agents, "default")
	assert.Contains(t, cfg.Providers, "openai")
	assert.Contains(t, cfg.Models, "gpt4o")
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestLoadTOML(t *testing.T) {
	content := `
[agents.myagent]
provider = "openai"
model    = "gpt4o"
system_prompt = "Be concise."

[providers.openai]
api_key_env = "OPENAI_API_KEY"

[models.gpt4o]
model_id    = "gpt-4o"
max_tokens  = 2048
temperature = 0.7
top_p       = 1.0
system_prompt = "You are a test model."
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	agent, err := cfg.GetAgent("myagent")
	require.NoError(t, err)
	assert.Equal(t, "openai", agent.Provider)
	assert.Equal(t, "gpt4o", agent.Model)
	assert.Equal(t, "Be concise.", agent.SystemPrompt)

	model, err := cfg.GetModel("gpt4o")
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", model.ModelID)
	assert.Equal(t, 2048, model.MaxTokens)
	assert.InDelta(t, 0.7, model.Temperature, 0.001)
	assert.Equal(t, "You are a test model.", model.SystemPrompt)
}

func TestLoadMCPConfig(t *testing.T) {
	content := `
[mcp_servers.test-stdio]
command = "echo"
args = ["hello"]

[mcp_servers.test-sse]
url = "http://localhost:8080/sse"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Contains(t, cfg.MCPServers, "test-stdio")
	assert.Equal(t, "echo", cfg.MCPServers["test-stdio"].Command)
	assert.Equal(t, []string{"hello"}, cfg.MCPServers["test-stdio"].Args)

	assert.Contains(t, cfg.MCPServers, "test-sse")
	assert.Equal(t, "http://localhost:8080/sse", cfg.MCPServers["test-sse"].URL)
}

func TestGetAgentNotFound(t *testing.T) {
	cfg, err := config.Load(tempConfigPath(t))
	require.NoError(t, err)
	_, err = cfg.GetAgent("nonexistent")
	assert.Error(t, err)
}

func TestGetModelNotFound(t *testing.T) {
	cfg, err := config.Load(tempConfigPath(t))
	require.NoError(t, err)
	_, err = cfg.GetModel("nonexistent")
	assert.Error(t, err)
}

func TestProviderAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key-123")
	cfg, err := config.Load(tempConfigPath(t))
	require.NoError(t, err)
	p, err := cfg.GetProvider("openai")
	require.NoError(t, err)
	assert.Equal(t, "test-key-123", p.APIKey())
}

func TestLoadDisableToolsConfig(t *testing.T) {
	content := `
disable_tools = true
[agents.test]
provider = "openai"
model = "gpt4o"
disable_tools = false
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.True(t, cfg.DisableTools)
	assert.False(t, cfg.Agents["test"].DisableTools)
}

func TestGetAgentDefaultFallback(t *testing.T) {
	content := `
[agents.z-agent]
provider = "openai"
model = "gpt4o"

[agents.a-agent]
provider = "anthropic"
model = "claude-sonnet"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	// Should fallback to a-agent (first sorted)
	a, err := cfg.GetAgent("")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", a.Provider)

	a, err = cfg.GetAgent("default")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", a.Provider)
}

func TestLoadZopInstructions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	zopPath := filepath.Join(dir, "ZOP.md")
	content := "Test instructions"
	require.NoError(t, os.WriteFile(zopPath, []byte(content), 0600))

	got, err := config.LoadZopInstructions(configPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// Test missing file
	got, err = config.LoadZopInstructions(filepath.Join(dir, "nonexistent", "config.toml"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func tempConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.toml")
}
