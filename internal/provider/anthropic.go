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
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
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
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	body := anthropicRequest{
		Model:     req.Model.ModelID,
		MaxTokens: req.Model.MaxTokens,
		System:    system,
		Messages:  msgs,
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

	for _, c := range aResp.Content {
		if c.Type == "text" {
			return CompletionResponse{Content: c.Text}, nil
		}
	}
	return CompletionResponse{}, fmt.Errorf("anthropic returned no text content")
}
