// Package config handles loading and validating zop configuration from TOML files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

# ─────────────────────────────────────────────
# Voice Output (TTS)
# ─────────────────────────────────────────────
[tts]
# model_url = "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-piper-en_US-amy-low.tar.bz2"
# model_name = "vits-piper-en_US-amy-low"
# piper_model = "en_US-amy-low.onnx"
speed = 1.5
safety_delay_ms = 10
`

// AgentConfig defines a named agent that pairs a provider with a model.
type AgentConfig struct {
	Provider     string      `toml:"provider"`
	Model        string      `toml:"model"`
	SystemPrompt string      `toml:"system_prompt"`
	ToolPolicy   *ToolPolicy `toml:"tool_policy,omitempty"`
	DisableTools bool        `toml:"disable_tools"`
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

// TTSConfig holds settings for text-to-speech output.
type TTSConfig struct {
	ModelURL      string  `toml:"model_url"`
	ModelName     string  `toml:"model_name"`
	PiperModel    string  `toml:"piper_model"`
	Speed         float32 `toml:"speed"`
	SafetyDelayMS int     `toml:"safety_delay_ms"`
}

// Config is the top-level configuration structure.
type Config struct {
	Agents       map[string]AgentConfig     `toml:"agents"`
	Providers    map[string]ProviderConfig  `toml:"providers"`
	Models       map[string]ModelConfig     `toml:"models"`
	MCPServers   map[string]MCPServerConfig `toml:"mcp_servers"`
	ToolPolicy   ToolPolicy                 `toml:"tool_policy"`
	DisableTools bool                       `toml:"disable_tools"`
	TTS          TTSConfig                  `toml:"tts"`
}

// ToolPolicy defines allowlist and denylist for tool calls.
type ToolPolicy struct {
	AllowList []ToolEntry `toml:"allow_list"`
	DenyList  []ToolEntry `toml:"deny_list"`
	AllowTags []string    `toml:"allow_tags"`
	DenyTags  []string    `toml:"deny_tags"`
}

// ToolEntry represents a pattern to match a tool call.
type ToolEntry struct {
	Tool       string   `toml:"tool,omitempty"`
	Exact      []string `toml:"exact,omitempty"`
	Regex      string   `toml:"regex,omitempty"`
	RegexArray []string `toml:"regex_array,omitempty"`
	Tags       []string `toml:"tags,omitempty"`
}

// MCPServerConfig defines an MCP server connection.
type MCPServerConfig struct {
	URL     string   `toml:"url"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
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

// LoadZopInstructions reads ZOP.md from the same directory as the config file.
func LoadZopInstructions(configPath string) (string, error) {
	if configPath == "" {
		configPath = DefaultConfigPath()
	}
	dir := filepath.Dir(configPath)
	path := filepath.Join(dir, "ZOP.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading ZOP.md: %w", err)
	}
	return string(data), nil
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

	// If the file contains an [agents] section, we clear the default agents.
	var raw map[string]interface{}
	if _, err := toml.Decode(string(data), &raw); err == nil {
		if _, hasAgents := raw["agents"]; hasAgents {
			cfg.Agents = make(map[string]AgentConfig)
		}
	}

	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}
	return cfg, nil
}

// GetAgent returns the AgentConfig for the named agent.
func (c *Config) GetAgent(name string) (AgentConfig, error) {
	if name == "" || name == "default" {
		if a, ok := c.Agents["default"]; ok {
			return a, nil
		}
		// Fallback to first agent found (sorted)
		names := c.SortedAgentNames()
		if len(names) > 0 {
			return c.Agents[names[0]], nil
		}
		if name == "" {
			name = "default"
		}
	}
	a, ok := c.Agents[name]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent %q not found in config", name)
	}
	return a, nil
}

// SortedAgentNames returns agent names in alphabetical order.
func (c *Config) SortedAgentNames() []string {
	keys := make([]string, 0, len(c.Agents))
	for k := range c.Agents {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	ensureSection(raw, "mcp_servers")
	ensureSection(raw, "tool_policy")

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
