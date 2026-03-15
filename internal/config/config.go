// Package config handles loading and validating zop configuration from TOML files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const defaultConfigTOML = `# zop default configuration file
# Copy to ~/.config/zop/config.toml and customize

# ─────────────────────────────────────────────
# Agents – each agent binds a provider + model
# ─────────────────────────────────────────────
[agents.default]
provider = "openai"
model    = "gpt4o"
# system_prompt = "You are a helpful assistant."

[agents.claude]
provider = "anthropic"
model    = "claude-sonnet"

[agents.gemini]
provider = "google"
model    = "gemini-pro"

[agents.openrouter]
provider = "openrouter"
model    = "openrouter-default"

[agents.ollama]
provider = "ollama"
model    = "llama3"

# ─────────────────────────────────────────────
# Providers
# ─────────────────────────────────────────────

[providers.openai]
api_key_env = "OPENAI_API_KEY"
# base_url  = "https://api.openai.com/v1"  # default; override if needed

[providers.anthropic]
api_key_env = "ANTHROPIC_API_KEY"
# base_url  = "https://api.anthropic.com"  # default

[providers.google]
api_key_env = "GOOGLE_API_KEY"
# base_url  = "https://generativelanguage.googleapis.com"  # default

[providers.openrouter]
api_key_env = "OPENROUTER_API_KEY"
base_url    = "https://openrouter.ai/api/v1"

[providers.ollama]
# No API key required for local Ollama
base_url = "http://localhost:11434/v1"

# ─────────────────────────────────────────────
# Models
# ─────────────────────────────────────────────

[models.gpt4o]
model_id    = "gpt-4o"
max_tokens  = 4096
temperature = 1.0
top_p       = 1.0
# system_prompt = "You are a helpful assistant."

[models.gpt4o-mini]
model_id    = "gpt-4o-mini"
max_tokens  = 4096
temperature = 1.0
top_p       = 1.0

[models.gpt35]
model_id    = "gpt-3.5-turbo"
max_tokens  = 2048
temperature = 1.0
top_p       = 1.0

[models.claude-sonnet]
model_id    = "claude-3-5-sonnet-20241022"
max_tokens  = 8192
temperature = 1.0
top_p       = 1.0

[models.claude-haiku]
model_id    = "claude-3-5-haiku-20241022"
max_tokens  = 8192
temperature = 1.0
top_p       = 1.0

[models.gemini-pro]
model_id    = "gemini-1.5-pro"
max_tokens  = 8192
temperature = 1.0
top_p       = 1.0

[models.openrouter-default]
model_id    = "openai/gpt-4o"
max_tokens  = 4096
temperature = 1.0
top_p       = 1.0

[models.llama3]
model_id    = "llama3"
max_tokens  = 4096
temperature = 0.8
top_p       = 0.95

[models.mistral]
model_id    = "mistral"
max_tokens  = 4096
temperature = 0.8
top_p       = 0.95
`

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
	// SystemPrompt defines a model-specific system prompt.
	SystemPrompt string `toml:"system_prompt"`
}

// Config is the top-level configuration structure.
type Config struct {
	Agents    map[string]AgentConfig    `toml:"agents"`
	Providers map[string]ProviderConfig `toml:"providers"`
	Models    map[string]ModelConfig    `toml:"models"`
}

// RawConfig is the untyped config structure used for editing config files.
type RawConfig map[string]map[string]map[string]interface{}

// DefaultConfigPath returns the OS-appropriate default config file path.
func DefaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "zop", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "zop", "config.toml")
}

// Load reads a TOML config file and returns a *Config.
// If path is empty the default path is used; if the file doesn't exist a
// default config file is created and loaded.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	cfg, err := defaultConfig()
	if err != nil {
		return nil, err
	}

	if _, err := EnsureConfigFile(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
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
func defaultConfig() (*Config, error) {
	cfg := &Config{}
	if _, err := toml.Decode(defaultConfigTOML, cfg); err != nil {
		return nil, fmt.Errorf("parsing default config: %w", err)
	}
	return cfg, nil
}

// EnsureConfigFile ensures a config file exists on disk and returns its path.
func EnsureConfigFile(path string) (string, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("checking config %q: %w", path, err)
	}

	if err := writeDefaultConfig(path); err != nil {
		return "", err
	}
	return path, nil
}

// LoadRaw reads the config file into an untyped structure for editing.
func LoadRaw(path string) (RawConfig, error) {
	path, err := EnsureConfigFile(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	raw := make(RawConfig)
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}

	ensureSection(raw, "agents")
	ensureSection(raw, "providers")
	ensureSection(raw, "models")

	return raw, nil
}

// WriteRaw writes an untyped config structure back to disk.
func WriteRaw(path string, raw RawConfig) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(raw); err != nil {
		return fmt.Errorf("encoding config %q: %w", path, err)
	}
	return os.WriteFile(path, buf.Bytes(), 0600)
}

func ensureSection(raw RawConfig, name string) {
	if raw[name] == nil {
		raw[name] = map[string]map[string]interface{}{}
	}
}

func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultConfigTOML), 0600); err != nil {
		return fmt.Errorf("writing default config %q: %w", path, err)
	}
	return nil
}
