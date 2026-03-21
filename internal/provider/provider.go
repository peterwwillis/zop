// Package provider defines the Provider interface and factory for AI backends.
package provider

import (
	"context"
	"fmt"

	"github.com/peterwwillis/zop/internal/config"
)

// Message represents a single conversation turn.
type Message struct {
	Role       string // "system", "user", "assistant", or "tool"
	Content    string
	ToolCalls  []ToolCall // For role "assistant"
	ToolID     string     // For role "tool"
}

// ToolCall represents a request from the model to call a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON-encoded arguments
}

// Tool defines a tool that can be called by the model.
type Tool struct {
	Name        string
	Description string
	Parameters  interface{} // JSON schema for parameters
}

// CompletionRequest contains all parameters for a completion call.
type CompletionRequest struct {
	Messages    []Message
	Model       config.ModelConfig
	Stream      bool
	StreamFunc  func(chunk string) // called for each streamed chunk when Stream is true
	Tools       []Tool
}

// CompletionResponse holds the model's reply.
type CompletionResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// Provider is the common interface that all AI backend implementations satisfy.
type Provider interface {
	// Complete sends a completion request and returns the model's response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	// Name returns the provider's identifier (e.g. "openai").
	Name() string
}

// New constructs the appropriate Provider for the given agent + config.
func New(agentName string, cfg *config.Config) (Provider, error) {
	agent, err := cfg.GetAgent(agentName)
	if err != nil {
		return nil, err
	}
	provCfg, err := cfg.GetProvider(agent.Provider)
	if err != nil {
		return nil, err
	}

	switch agent.Provider {
	case "openai", "openrouter", "ollama":
		return newOpenAICompatible(agent.Provider, provCfg)
	case "anthropic":
		return newAnthropic(provCfg)
	case "google":
		return newGoogle(provCfg)
	default:
		return nil, fmt.Errorf("unknown provider %q", agent.Provider)
	}
}
