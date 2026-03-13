package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peterwwillis/pgpt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Contains(t, cfg.Agents, "default")
	assert.Contains(t, cfg.Providers, "openai")
	assert.Contains(t, cfg.Models, "gpt4o")
}

func TestLoadTOML(t *testing.T) {
	content := `
[agents.myagent]
provider = "openai"
model    = "gpt4o"

[providers.openai]
api_key_env = "OPENAI_API_KEY"

[models.gpt4o]
model_id    = "gpt-4o"
max_tokens  = 2048
temperature = 0.7
top_p       = 1.0
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

	model, err := cfg.GetModel("gpt4o")
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", model.ModelID)
	assert.Equal(t, 2048, model.MaxTokens)
	assert.InDelta(t, 0.7, model.Temperature, 0.001)
}

func TestGetAgentNotFound(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	_, err = cfg.GetAgent("nonexistent")
	assert.Error(t, err)
}

func TestGetModelNotFound(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	_, err = cfg.GetModel("nonexistent")
	assert.Error(t, err)
}

func TestProviderAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key-123")
	cfg, err := config.Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	p, err := cfg.GetProvider("openai")
	require.NoError(t, err)
	assert.Equal(t, "test-key-123", p.APIKey())
}

func TestDefaultConfigPath(t *testing.T) {
	path := config.DefaultConfigPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "pgpt")
}
