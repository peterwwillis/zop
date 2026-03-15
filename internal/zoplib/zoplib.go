// Package zoplib exposes the core zop AI functionality as a mobile library.
// It is designed to be bound to Android/iOS via gomobile bind.
//
// gomobile generates Java bindings where the package name ("zoplib") becomes
// both the Java package and the class prefix.  The exported Go function
// Query becomes the static Java method zoplib.Zoplib.query(...).
package zoplib

import (
	"context"
	"fmt"
	"net/http"

	openai "github.com/sashabaranov/go-openai"
)

// Query sends userPrompt to the specified OpenAI-compatible API endpoint and
// returns the assistant's response text.
//
// Parameters:
//   - apiKey:       authentication key for the AI provider.
//   - baseURL:      base URL of the API (e.g. "https://api.openai.com/v1").
//                   Pass an empty string to use the OpenAI default.
//   - model:        model identifier (e.g. "gpt-4o").
//   - systemPrompt: optional system message; pass an empty string to omit.
//   - userPrompt:   the user's message.
//
// On error, the returned error message is forwarded to the Java caller as a
// standard Java Exception (gomobile translates Go errors automatically).
func Query(apiKey, baseURL, model, systemPrompt, userPrompt string) (string, error) {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	cfg.HTTPClient = &http.Client{}
	client := openai.NewClientWithConfig(cfg)

	messages := make([]openai.ChatCompletionMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return resp.Choices[0].Message.Content, nil
}
