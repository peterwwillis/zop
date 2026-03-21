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
	Contents         []googleContent         `json:"contents"`
	SystemInstruction *googleContent          `json:"systemInstruction,omitempty"`
	GenerationConfig *googleGenerationConfig `json:"generationConfig,omitempty"`
	Tools            []googleTool            `json:"tools,omitempty"`
}

type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"function_declarations,omitempty"`
}

type googleFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text         string              `json:"text,omitempty"`
	FunctionCall *googleFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

type googleFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type googleFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
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
		if m.Role == "system" {
			systemInstruction = &googleContent{
				Parts: []googlePart{{Text: m.Content}},
			}
			continue
		}

		var parts []googlePart
		role := m.Role
		switch m.Role {
		case "user":
			parts = append(parts, googlePart{Text: m.Content})
		case "assistant":
			role = "model"
			if m.Content != "" {
				parts = append(parts, googlePart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				parts = append(parts, googlePart{
					FunctionCall: &googleFunctionCall{Name: tc.Name, Args: args},
				})
			}
		case "tool":
			role = "function" // Google uses 'function' role for tool results in some API versions, but usually it's model -> functionResponse
			// Actually, Google's structure is a bit different. Tool results go into 'parts' with 'functionResponse'.
			var resp map[string]interface{}
			_ = json.Unmarshal([]byte(m.Content), &resp)
			parts = append(parts, googlePart{
				FunctionResponse: &googleFunctionResponse{Name: m.ToolID, Response: resp},
			})
		}
		contents = append(contents, googleContent{Role: role, Parts: parts})
	}

	var funcDecls []googleFunctionDeclaration
	for _, t := range req.Tools {
		funcDecls = append(funcDecls, googleFunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
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
	if len(funcDecls) > 0 {
		gReq.Tools = []googleTool{{FunctionDeclarations: funcDecls}}
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

	var result CompletionResponse
	for _, candidate := range gResp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				result.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsData, _ := json.Marshal(part.FunctionCall.Args)
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					ID:        part.FunctionCall.Name, // Gemini doesn't always have IDs, uses Name
					Name:      part.FunctionCall.Name,
					Arguments: string(argsData),
				})
			}
		}
	}
	if result.Content == "" && len(result.ToolCalls) == 0 {
		return CompletionResponse{}, fmt.Errorf("google returned no content")
	}
	return result, nil
}
