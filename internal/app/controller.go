package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/peterwwillis/zop/internal/chat"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/peterwwillis/zop/internal/provider"
)

const (
	defaultSessionName          = "mobile"
	defaultWhisperModelFilename = "ggml-base.en.bin"
)

// Controller coordinates config, providers, and chat history for the mobile UI.
type Controller struct {
	mu             sync.Mutex
	cfg            *config.Config
	configPath     string
	agentName      string
	modelConfig    config.ModelConfig
	providerConfig config.ProviderConfig
	prov           provider.Provider
	systemPrompt   string
	messages       []provider.Message
	sessionMgr     *chat.Manager
	sessionBase    string
}

// NewController loads configuration and prepares a provider instance.
func NewController(configPath, sessionName, agentName string) (*Controller, error) {
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}
	if sessionName == "" {
		sessionName = defaultSessionName
	}

	if err := ensureWhisperModelPath(); err != nil {
		return nil, err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if agentName == "" {
		agentName = defaultAgentName(cfg)
	}
	ctrl := &Controller{
		cfg:         cfg,
		configPath:  configPath,
		agentName:   agentName,
		sessionBase: sessionName,
	}

	sessionMgr, err := chat.NewManager("")
	if err != nil {
		return nil, err
	}
	ctrl.sessionMgr = sessionMgr

	if err := ctrl.reloadProviderLocked(); err != nil {
		return nil, err
	}
	return ctrl, nil
}

// ConfigPath returns the active config file path.
func (c *Controller) ConfigPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.configPath
}

// AgentNames returns available agent names in sorted order.
func (c *Controller) AgentNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return sortedAgentNames(c.cfg)
}

// ActiveAgent returns the currently selected agent name.
func (c *Controller) ActiveAgent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentName
}

// MissingAPIKeyWarning returns a warning string if the provider expects an API key.
func (c *Controller) MissingAPIKeyWarning() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.providerConfig.APIKeyEnv != "" && c.providerConfig.APIKey() == "" {
		return fmt.Sprintf("warning: environment variable %s is not set; API requests will fail", c.providerConfig.APIKeyEnv)
	}
	return ""
}

// Messages returns a copy of the current message history.
func (c *Controller) Messages() []provider.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]provider.Message(nil), c.messages...)
}

// ReloadConfig reloads config from disk and refreshes the provider.
func (c *Controller) ReloadConfig() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg, err := config.Load(c.configPath)
	if err != nil {
		return err
	}
	c.cfg = cfg
	if _, ok := c.cfg.Agents[c.agentName]; !ok {
		c.agentName = defaultAgentName(c.cfg)
	}
	return c.reloadProviderLocked()
}

// SetAgent updates the active agent and refreshes the provider.
func (c *Controller) SetAgent(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cfg.Agents[name]; !ok {
		return fmt.Errorf("agent %q not found in config", name)
	}
	if c.agentName == name {
		return nil
	}
	c.agentName = name
	return c.reloadProviderLocked()
}

// ClearSession clears the session history on disk and in memory.
func (c *Controller) ClearSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionMgr != nil {
		if err := c.sessionMgr.Delete(c.sessionNameLocked()); err != nil {
			return err
		}
	}
	c.messages = nil
	if c.systemPrompt != "" {
		c.messages = append(c.messages, provider.Message{Role: "system", Content: c.systemPrompt})
	}
	return nil
}

// SendPrompt sends a prompt to the provider and persists chat history.
func (c *Controller) SendPrompt(ctx context.Context, prompt string, streamFunc func(string)) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", nil
	}

	c.mu.Lock()
	messages := append([]provider.Message(nil), c.messages...)
	modelCfg := c.modelConfig
	prov := c.prov
	sessionMgr := c.sessionMgr
	sessionName := c.sessionNameLocked()
	c.mu.Unlock()

	messages = append(messages, provider.Message{Role: "user", Content: prompt})
	req := provider.CompletionRequest{
		Messages:   messages,
		Model:      modelCfg,
		Stream:     streamFunc != nil,
		StreamFunc: streamFunc,
	}
	resp, err := prov.Complete(ctx, req)
	if err != nil {
		return "", err
	}
	messages = append(messages, provider.Message{Role: "assistant", Content: resp.Content})

	c.mu.Lock()
	c.messages = messages
	c.mu.Unlock()

	if sessionMgr != nil && sessionName != "" {
		if err := sessionMgr.Save(sessionName, messages); err != nil {
			return resp.Content, err
		}
	}
	return resp.Content, nil
}

func (c *Controller) reloadProviderLocked() error {
	agent, err := c.cfg.GetAgent(c.agentName)
	if err != nil {
		return err
	}
	modelCfg, err := c.cfg.GetModel(agent.Model)
	if err != nil {
		return err
	}
	provCfg, err := c.cfg.GetProvider(agent.Provider)
	if err != nil {
		return err
	}
	prov, err := provider.New(c.agentName, c.cfg)
	if err != nil {
		return err
	}

	c.modelConfig = modelCfg
	c.providerConfig = provCfg
	c.prov = prov
	c.systemPrompt = ""
	if agent.SystemPrompt != "" {
		c.systemPrompt = agent.SystemPrompt
	} else if modelCfg.SystemPrompt != "" {
		c.systemPrompt = modelCfg.SystemPrompt
	}

	return c.loadHistoryLocked()
}

func (c *Controller) loadHistoryLocked() error {
	if c.sessionMgr == nil {
		c.messages = nil
		return nil
	}
	history, err := c.sessionMgr.Get(c.sessionNameLocked())
	if err != nil {
		return err
	}
	if len(history) > 0 {
		c.messages = history
		return nil
	}
	c.messages = nil
	if c.systemPrompt != "" {
		c.messages = append(c.messages, provider.Message{Role: "system", Content: c.systemPrompt})
	}
	return nil
}

func (c *Controller) sessionNameLocked() string {
	base := c.sessionBase
	if base == "" || chat.ValidateName(base) != nil {
		base = defaultSessionName
	}
	if c.agentName == "" {
		return base
	}
	candidate := fmt.Sprintf("%s-%s", base, c.agentName)
	if chat.ValidateName(candidate) != nil {
		return base
	}
	return candidate
}

func defaultAgentName(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if _, ok := cfg.Agents["default"]; ok {
		return "default"
	}
	names := sortedAgentNames(cfg)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func sortedAgentNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ensureWhisperModelPath() error {
	if _, ok := os.LookupEnv("ZOP_WHISPER_MODEL"); ok {
		return nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolving config dir: %w", err)
	}
	modelPath := filepath.Join(configDir, "zop", "whisper", defaultWhisperModelFilename)
	return os.Setenv("ZOP_WHISPER_MODEL", modelPath)
}
