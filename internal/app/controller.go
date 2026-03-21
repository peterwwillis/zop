package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/peterwwillis/zop/internal/chat"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/peterwwillis/zop/internal/mcp"
	"github.com/peterwwillis/zop/internal/provider"
	"github.com/peterwwillis/zop/internal/tool"
	"github.com/peterwwillis/zop/internal/tts"
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
	speaker        tts.Speaker
	toolRegistry   *tool.Registry
	toolsEnabled   bool
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

	speaker, err := tts.NewSpeaker(cfg.TTS)
	if err != nil {
		return nil, fmt.Errorf("initializing TTS speaker: %w", err)
	}

	ctrl := &Controller{
		cfg:          cfg,
		configPath:   configPath,
		agentName:    agentName,
		sessionBase:  sessionName,
		speaker:      speaker,
		toolRegistry: tool.NewRegistry(),
	}

	// Register built-in tools
	ctrl.toolRegistry.Register(&tool.RunCommandTool{})

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
	if c.cfg == nil {
		return nil
	}
	return c.cfg.SortedAgentNames()
}

// ActiveAgent returns the currently selected agent name.
func (c *Controller) ActiveAgent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentName
}

// SetProvider overrides the current provider. (Primarily for testing)
func (c *Controller) SetProvider(p provider.Provider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prov = p
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

	if c.speaker != nil {
		_ = c.speaker.Close()
	}
	speaker, err := tts.NewSpeaker(c.cfg.TTS)
	if err != nil {
		return fmt.Errorf("re-initializing TTS speaker: %w", err)
	}
	c.speaker = speaker

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

// TTSConfig returns the current TTS configuration.
func (c *Controller) TTSConfig() config.TTSConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.TTS
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
	return c.resetMessagesLocked()
}

func (c *Controller) resetMessagesLocked() error {
	c.messages = nil
	if c.systemPrompt != "" {
		c.messages = append(c.messages, provider.Message{Role: "system", Content: c.systemPrompt})
	}

	zopInstructions, err := config.LoadZopInstructions(c.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load ZOP.md: %v\n", err)
	} else if zopInstructions != "" {
		c.messages = append(c.messages, provider.Message{Role: "system", Content: zopInstructions})
	}

	if c.toolsEnabled {
		c.messages = append(c.messages, provider.Message{
			Role:    "system",
			Content: "You have access to tools. Use them ONLY when explicitly required by the user's request or necessary to fulfill it. If you can provide a high-quality response without tools, do so.",
		})
	}
	return nil
}

// SendPrompt sends a prompt to the provider and persists chat history.
// It handles tool calling loops.
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
	registry := c.toolRegistry
	toolsEnabled := c.toolsEnabled
	c.mu.Unlock()

	messages = append(messages, provider.Message{Role: "user", Content: prompt})

	var lastContent string
	for {
		var tools []provider.Tool
		if toolsEnabled {
			tools = registry.List()
		}
		req := provider.CompletionRequest{
			Messages:   messages,
			Model:      modelCfg,
			Stream:     streamFunc != nil,
			StreamFunc: streamFunc,
			Tools:      tools,
		}
		resp, err := prov.Complete(ctx, req)
		if err != nil {
			return "", err
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		lastContent = resp.Content

		if len(resp.ToolCalls) == 0 {
			break
		}

		// Execute tool calls
		for _, tc := range resp.ToolCalls {
			t, ok := registry.Get(tc.Name)
			var toolResult string
			if !ok {
				toolResult = fmt.Sprintf("Error: tool %q not found", tc.Name)
			} else {
				res, err := t.Execute(ctx, tc.Arguments)
				if err != nil {
					toolResult = fmt.Sprintf("Error: %v", err)
				} else {
					toolResult = res
				}
			}
			messages = append(messages, provider.Message{
				Role:    "tool",
				ToolID:  tc.ID,
				Content: toolResult,
			})
		}
		// Stream a separator if we are streaming
		if streamFunc != nil {
			streamFunc("\n[tool calling...]\n")
		}
	}

	c.mu.Lock()
	c.messages = messages
	c.mu.Unlock()

	if sessionMgr != nil && sessionName != "" {
		if err := sessionMgr.Save(sessionName, messages); err != nil {
			return lastContent, err
		}
	}
	return lastContent, nil
}

// Speak converts text to speech and plays it.
func (c *Controller) Speak(ctx context.Context, text string) error {
	c.mu.Lock()
	speaker := c.speaker
	c.mu.Unlock()

	if speaker == nil {
		return fmt.Errorf("voice output is not enabled")
	}
	return speaker.Speak(ctx, text)
}

// WaitSpeaker waits for voice output to finish.
func (c *Controller) WaitSpeaker() error {
	c.mu.Lock()
	speaker := c.speaker
	c.mu.Unlock()

	if speaker != nil {
		return speaker.Wait()
	}
	return nil
}

// Close releases controller resources.
func (c *Controller) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.speaker != nil {
		return c.speaker.Close()
	}
	return nil
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

	// Reload tools/MCP
	c.toolRegistry = tool.NewRegistry()
	policy := c.cfg.ToolPolicy
	if agent.ToolPolicy != nil {
		policy = *agent.ToolPolicy
	}
	c.toolsEnabled = !agent.DisableTools && !c.cfg.DisableTools && len(policy.AllowList) > 0

	c.toolRegistry.Register(&tool.RunCommandTool{
		Policy: tool.NewPolicyChecker(policy),
	})
	checker := tool.NewPolicyChecker(policy)
	for name, mcpCfg := range c.cfg.MCPServers {
		mcpClient, err := mcp.NewClient(context.Background(), mcpCfg.URL, mcpCfg.Command, mcpCfg.Args...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to connect to MCP server %q: %v\n", name, err)
			continue
		}
		wrappers, err := mcp.WrapTools(context.Background(), mcpClient, checker)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list tools for MCP server %q: %v\n", name, err)
			continue
		}
		for _, w := range wrappers {
			c.toolRegistry.Register(w)
		}
	}

	return c.loadHistoryLocked()
}

func (c *Controller) loadHistoryLocked() error {
	if c.sessionMgr == nil {
		return c.resetMessagesLocked()
	}
	history, err := c.sessionMgr.Get(c.sessionNameLocked())
	if err != nil {
		return err
	}
	if len(history) > 0 {
		c.messages = history
		return nil
	}
	return c.resetMessagesLocked()
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
	names := cfg.SortedAgentNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
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
