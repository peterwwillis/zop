package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/peterwwillis/zop/internal/config"
)

const anthropicDefaultBase = "https://api.anthropic.com"
const anthropicAPIVersion = "2023-06-01"

type anthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func newAnthropic(provCfg config.ProviderConfig) (*anthropicProvider, error) {
	base := provCfg.BaseURL
	if base == "" {
		base = anthropicDefaultBase
	}
	return &anthropicProvider{
		apiKey:  provCfg.APIKey(),
		baseURL: base,
		client:  &http.Client{},
	}, nil
}

func (p *anthropicProvider) Name() string { return "anthropic" }

// anthropicRequest is the JSON body for POST /v1/messages.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	ID     string `json:"id,omitempty"`      // For tool_use and tool_result
	Name   string `json:"name,omitempty"`    // For tool_use
	Input  json.RawMessage `json:"input,omitempty"`   // For tool_use
	ToolUseID string `json:"tool_use_id,omitempty"` // For tool_result
	Content   string `json:"content,omitempty"`      // For tool_result
}

type anthropicResponse struct {
	Content []anthropicBlock `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *anthropicProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	// Split system messages from conversation messages.
	var system string
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}

		var blocks []anthropicBlock
		switch m.Role {
		case "assistant":
			if m.Content != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Arguments),
				})
			}
		case "tool":
			blocks = append(blocks, anthropicBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolID,
				Content:   m.Content,
			})
		default:
			blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
		}
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: blocks})
	}

	var tools []anthropicTool
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	body := anthropicRequest{
		Model:     req.Model.ModelID,
		MaxTokens: req.Model.MaxTokens,
		System:    system,
		Messages:  msgs,
		Tools:     tools,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic read body: %w", err)
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(raw, &aResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic unmarshal: %w", err)
	}

	if aResp.Error != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic API error (%s): %s", aResp.Error.Type, aResp.Error.Message)
	}

	var result CompletionResponse
	for _, c := range aResp.Content {
		switch c.Type {
		case "text":
			result.Content += c.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        c.ID,
				Name:      c.Name,
				Arguments: string(c.Input),
			})
		}
	}
	if result.Content == "" && len(result.ToolCalls) == 0 {
		return CompletionResponse{}, fmt.Errorf("anthropic returned no content")
	}
	return result, nil
}
