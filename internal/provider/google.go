package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/peterwwillis/pgpt/internal/config"
)

const googleDefaultBase = "https://generativelanguage.googleapis.com"

type googleProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func newGoogle(provCfg config.ProviderConfig) (*googleProvider, error) {
	base := provCfg.BaseURL
	if base == "" {
		base = googleDefaultBase
	}
	return &googleProvider{
		apiKey:  provCfg.APIKey(),
		baseURL: base,
		client:  &http.Client{},
	}, nil
}

func (p *googleProvider) Name() string { return "google" }

// Google Gemini generateContent types.
type googleRequest struct {
	Contents         []googleContent       `json:"contents"`
	SystemInstruction *googleContent        `json:"systemInstruction,omitempty"`
	GenerationConfig *googleGenerationConfig `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float32 `json:"temperature,omitempty"`
	TopP            float32 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
}

type googleResponse struct {
	Candidates []struct {
		Content googleContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (p *googleProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	var systemInstruction *googleContent
	var contents []googleContent

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			systemInstruction = &googleContent{
				Parts: []googlePart{{Text: m.Content}},
			}
		case "user":
			contents = append(contents, googleContent{
				Role:  "user",
				Parts: []googlePart{{Text: m.Content}},
			})
		case "assistant":
			contents = append(contents, googleContent{
				Role:  "model",
				Parts: []googlePart{{Text: m.Content}},
			})
		}
	}

	gReq := googleRequest{
		Contents:         contents,
		SystemInstruction: systemInstruction,
		GenerationConfig: &googleGenerationConfig{
			MaxOutputTokens: req.Model.MaxTokens,
			Temperature:     req.Model.Temperature,
			TopP:            req.Model.TopP,
			TopK:            req.Model.TopK,
		},
	}

	data, err := json.Marshal(gReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("google marshal: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		p.baseURL, req.Model.ModelID, p.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("google request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("google http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("google read body: %w", err)
	}

	var gResp googleResponse
	if err := json.Unmarshal(raw, &gResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("google unmarshal: %w", err)
	}

	if gResp.Error != nil {
		return CompletionResponse{}, fmt.Errorf("google API error (%s): %s", gResp.Error.Status, gResp.Error.Message)
	}

	for _, candidate := range gResp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				return CompletionResponse{Content: part.Text}, nil
			}
		}
	}
	return CompletionResponse{}, fmt.Errorf("google returned no text content")
}
