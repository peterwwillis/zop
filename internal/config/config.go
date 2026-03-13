// Package config handles loading and validating pgpt configuration from TOML files.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// AgentConfig defines a named agent that pairs a provider with a model.
type AgentConfig struct {
	Provider     string `toml:"provider"`
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
}

// ProviderConfig holds connection settings for an AI provider.
type ProviderConfig struct {
	// APIKeyEnv is the environment variable that contains the API key.
	APIKeyEnv string `toml:"api_key_env"`
	// BaseURL overrides the provider's default API endpoint.
	BaseURL string `toml:"base_url"`
}

// ModelConfig describes a model and its generation hyperparameters.
type ModelConfig struct {
	// ModelID is the provider-specific model identifier (e.g. "gpt-4o").
	ModelID     string  `toml:"model_id"`
	MaxTokens   int     `toml:"max_tokens"`
	Temperature float32 `toml:"temperature"`
	TopP        float32 `toml:"top_p"`
	// TopK is used by some providers (e.g. Google).
	TopK int `toml:"top_k"`
	// RepeatPenalty / frequency_penalty (OpenAI) / repetition_penalty.
	RepeatPenalty float32 `toml:"repeat_penalty"`
}

// Config is the top-level configuration structure.
type Config struct {
	Agents    map[string]AgentConfig    `toml:"agents"`
	Providers map[string]ProviderConfig `toml:"providers"`
	Models    map[string]ModelConfig    `toml:"models"`
}

// DefaultConfigPath returns the OS-appropriate default config file path.
func DefaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "pgpt", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "pgpt", "config.toml")
}

// Load reads a TOML config file and returns a *Config.
// If path is empty the default path is used; if the file doesn't exist a
// built-in default config is returned.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}
	return cfg, nil
}

// GetAgent returns the AgentConfig for the named agent.
func (c *Config) GetAgent(name string) (AgentConfig, error) {
	if name == "" {
		name = "default"
	}
	a, ok := c.Agents[name]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent %q not found in config", name)
	}
	return a, nil
}

// GetProvider returns the ProviderConfig for the named provider.
func (c *Config) GetProvider(name string) (ProviderConfig, error) {
	p, ok := c.Providers[name]
	if !ok {
		return ProviderConfig{}, fmt.Errorf("provider %q not found in config", name)
	}
	return p, nil
}

// GetModel returns the ModelConfig for the named model.
func (c *Config) GetModel(name string) (ModelConfig, error) {
	m, ok := c.Models[name]
	if !ok {
		return ModelConfig{}, fmt.Errorf("model %q not found in config", name)
	}
	return m, nil
}

// APIKey resolves the API key for p from the environment.
func (p ProviderConfig) APIKey() string {
	if p.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(p.APIKeyEnv)
}

// defaultConfig returns a Config pre-populated with sensible defaults so the
// tool is usable without any config file.
func defaultConfig() *Config {
	return &Config{
		Agents: map[string]AgentConfig{
			"default": {
				Provider: "openai",
				Model:    "gpt4o",
			},
		},
		Providers: map[string]ProviderConfig{
			"openai": {
				APIKeyEnv: "OPENAI_API_KEY",
			},
			"anthropic": {
				APIKeyEnv: "ANTHROPIC_API_KEY",
			},
			"google": {
				APIKeyEnv: "GOOGLE_API_KEY",
			},
			"openrouter": {
				APIKeyEnv: "OPENROUTER_API_KEY",
				BaseURL:   "https://openrouter.ai/api/v1",
			},
			"ollama": {
				BaseURL: "http://localhost:11434/v1",
			},
		},
		Models: map[string]ModelConfig{
			"gpt4o": {
				ModelID:     "gpt-4o",
				MaxTokens:   4096,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"gpt4o-mini": {
				ModelID:     "gpt-4o-mini",
				MaxTokens:   4096,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"gpt35": {
				ModelID:     "gpt-3.5-turbo",
				MaxTokens:   2048,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"claude-sonnet": {
				ModelID:     "claude-3-5-sonnet-20241022",
				MaxTokens:   8192,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"claude-haiku": {
				ModelID:     "claude-3-5-haiku-20241022",
				MaxTokens:   8192,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"gemini-pro": {
				ModelID:     "gemini-1.5-pro",
				MaxTokens:   8192,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"openrouter-default": {
				ModelID:     "openai/gpt-4o",
				MaxTokens:   4096,
				Temperature: 1.0,
				TopP:        1.0,
			},
			"llama3": {
				ModelID:     "llama3",
				MaxTokens:   4096,
				Temperature: 0.8,
				TopP:        0.95,
			},
		},
	}
}
