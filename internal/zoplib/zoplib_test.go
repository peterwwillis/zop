package zoplib

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// mockResponse returns a handler that always sends the given ChatCompletionResponse.
func mockResponse(t *testing.T, resp openai.ChatCompletionResponse) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("mock server encode: %v", err)
		}
	}
}

func TestQuery_Success(t *testing.T) {
	ts := httptest.NewServer(mockResponse(t, openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Hello from Zop!",
			}},
		},
	}))
	defer ts.Close()

	got, err := Query("test-key", ts.URL, "gpt-4o-mini", "", "Hello")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got != "Hello from Zop!" {
		t.Errorf("Query() = %q, want %q", got, "Hello from Zop!")
	}
}

func TestQuery_WithSystemPrompt(t *testing.T) {
	var capturedMessages []openai.ChatCompletionMessage

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		capturedMessages = req.Messages

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openai.ChatCompletionResponse{ //nolint:errcheck
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "ok"}},
			},
		})
	}))
	defer ts.Close()

	if _, err := Query("test-key", ts.URL, "gpt-4o-mini", "Be helpful", "Hello"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(capturedMessages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("first message role = %q, want %q", capturedMessages[0].Role, openai.ChatMessageRoleSystem)
	}
	if capturedMessages[1].Role != openai.ChatMessageRoleUser {
		t.Errorf("second message role = %q, want %q", capturedMessages[1].Role, openai.ChatMessageRoleUser)
	}
}

func TestQuery_NoSystemPrompt(t *testing.T) {
	var capturedMessages []openai.ChatCompletionMessage

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		capturedMessages = req.Messages

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openai.ChatCompletionResponse{ //nolint:errcheck
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "ok"}},
			},
		})
	}))
	defer ts.Close()

	if _, err := Query("test-key", ts.URL, "gpt-4o-mini", "", "Hello"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(capturedMessages) != 1 {
		t.Fatalf("expected 1 message (user only), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("message role = %q, want %q", capturedMessages[0].Role, openai.ChatMessageRoleUser)
	}
}

func TestQuery_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, err := Query("test-key", ts.URL, "gpt-4o-mini", "", "Hello")
	if err == nil {
		t.Error("Query() expected error on server error, got nil")
	}
}

func TestQuery_NoChoices(t *testing.T) {
	ts := httptest.NewServer(mockResponse(t, openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{},
	}))
	defer ts.Close()

	_, err := Query("test-key", ts.URL, "gpt-4o-mini", "", "Hello")
	if err == nil {
		t.Error("Query() expected error when no choices returned, got nil")
	}
}
