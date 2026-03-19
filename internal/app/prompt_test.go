package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptTemplates(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	configContent := `
[templates.summarizer]
system_prompt = "You are a summarizer assistant."
prompt = "Summarize the following: {{.Input}}"

[agents.default]
provider = "openai"
model = "gpt4o"

[agents.sum]
provider = "openai"
model = "gpt4o"
prompt_template = "summarizer"

[agents.env_agent]
provider = "openai"
model = "gpt4o"
system_prompt = "Hello {{.Env.ZOP_TEST_USER}}"

[providers.openai]
api_key_env = "OPENAI_API_KEY"

[models.gpt4o]
model_id = "gpt-4o"
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	t.Setenv("ZOP_TEST_USER", "Alice")
	t.Setenv("OPENAI_API_KEY", "fake-key")

	t.Run("DefaultAgentNoTemplate", func(t *testing.T) {
		ctrl, err := NewController(configPath, "test-session", "default")
		require.NoError(t, err)

		// User input as template
		prompt := "Tell me the date: {{date}}"
		finalPrompt, err := ctrl.executeTemplate("", prompt)
		require.NoError(t, err)
		assert.Contains(t, finalPrompt, "Tell me the date: 20") // Should contain current year
	})

	t.Run("SummarizerTemplate", func(t *testing.T) {
		ctrl, err := NewController(configPath, "test-session", "sum")
		require.NoError(t, err)

		tmplStr, err := ctrl.resolveUserPromptTemplate()
		require.NoError(t, err)
		assert.Equal(t, "Summarize the following: {{.Input}}", tmplStr)

		finalPrompt, err := ctrl.executeTemplate(tmplStr, "The quick brown fox.")
		require.NoError(t, err)
		assert.Equal(t, "Summarize the following: The quick brown fox.", finalPrompt)
	})

	t.Run("EnvSystemPrompt", func(t *testing.T) {
		ctrl, err := NewController(configPath, "test-session", "env_agent")
		require.NoError(t, err)
		assert.Equal(t, "Hello Alice", ctrl.systemPrompt)
	})
}

func TestPromptTemplateFiles(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	tmplPath := filepath.Join(configDir, "my.tmpl")
	err := os.WriteFile(tmplPath, []byte("File template for {{.Input}}"), 0600)
	require.NoError(t, err)

	configContent := `
[agents.file_agent]
provider = "openai"
model = "gpt4o"
prompt_file = "my.tmpl"

[providers.openai]
api_key_env = "OPENAI_API_KEY"

[models.gpt4o]
model_id = "gpt-4o"
`
	err = os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	t.Setenv("OPENAI_API_KEY", "fake-key")

	ctrl, err := NewController(configPath, "test-session", "file_agent")
	require.NoError(t, err)

	tmplStr, err := ctrl.resolveUserPromptTemplate()
	require.NoError(t, err)
	assert.Equal(t, "File template for {{.Input}}", tmplStr)

	finalPrompt, err := ctrl.executeTemplate(tmplStr, "data")
	require.NoError(t, err)
	assert.Equal(t, "File template for data", finalPrompt)
}
