package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"

	openai "github.com/sashabaranov/go-openai"

	"github.com/peterwwillis/zop/internal/config"
)

// openAICompatibleProvider handles OpenAI, OpenRouter, and Ollama backends
// all of which expose an OpenAI-compatible REST API.
type openAICompatibleProvider struct {
	name   string
	client *openai.Client
}

func newOpenAICompatible(name string, provCfg config.ProviderConfig) (*openAICompatibleProvider, error) {
	cfg := openai.DefaultConfig(provCfg.APIKey())
	if provCfg.BaseURL != "" {
		cfg.BaseURL = provCfg.BaseURL
	}
	cfg.HTTPClient = &http.Client{}
	return &openAICompatibleProvider{
		name:   name,
		client: openai.NewClientWithConfig(cfg),
	}, nil
}

func (p *openAICompatibleProvider) Name() string { return p.name }

func (p *openAICompatibleProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}
	}

	oaiReq := openai.ChatCompletionRequest{
		Model:       req.Model.ModelID,
		Messages:    msgs,
		MaxTokens:   req.Model.MaxTokens,
		Temperature: req.Model.Temperature,
		TopP:        req.Model.TopP,
		Stream:      req.Stream,
	}

	if req.Model.RepeatPenalty != 0 {
		oaiReq.FrequencyPenalty = req.Model.RepeatPenalty
	}

	if req.Stream && req.StreamFunc != nil {
		return p.streamCompletion(ctx, oaiReq, req.StreamFunc)
	}
	return p.syncCompletion(ctx, oaiReq)
}

func (p *openAICompatibleProvider) syncCompletion(ctx context.Context, req openai.ChatCompletionRequest) (CompletionResponse, error) {
	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("openai returned no choices")
	}
	return CompletionResponse{Content: resp.Choices[0].Message.Content}, nil
}

func (p *openAICompatibleProvider) streamCompletion(ctx context.Context, req openai.ChatCompletionRequest, fn func(string)) (CompletionResponse, error) {
	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai stream: %w", err)
	}
	defer stream.Close()

	var full string
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return CompletionResponse{}, fmt.Errorf("openai stream recv: %w", err)
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			full += delta
			fn(delta)
		}
	}
	return CompletionResponse{Content: full}, nil
}
