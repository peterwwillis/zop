package tts

import (
	"reflect"
	"strings"
	"testing"
)

func TestSanitizeMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // Check if it contains these strings
	}{
		{
			name:     "basic markdown",
			input:    "**bold** and *italic* and `code`",
			expected: []string{"bold", "italic", "code"},
		},
		{
			name:     "headers and lists",
			input:    "# Header\n- Item 1\n- Item 2\n1. Numbered",
			expected: []string{"Header", "Item 1", "Item 2", "Numbered"},
		},
		{
			name:     "links",
			input:    "Check this [link](https://example.com) and <https://example.com>",
			expected: []string{"Check this link", "and"},
		},
		{
			name:     "code blocks",
			input:    "Some text\n```go\nfmt.Println(\"Hello\")\n```\nMore text",
			expected: []string{"Some text", "More text"},
		},
		{
			name:     "complex markdown",
			input:    "## Section\n> Quote block\n\n- [ ] Task\n***\nFinal line.",
			expected: []string{"Section", "Quote block", "Task", "Final line"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMarkdown(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(got, exp) {
					t.Errorf("sanitizeMarkdown() = %q, want it to contain %q", got, exp)
				}
			}
			// Check that markdown characters are gone
			for _, char := range []string{"**", "__", "```", "[ ]"} {
				if strings.Contains(got, char) {
					t.Errorf("sanitizeMarkdown() = %q, still contains markdown char %q", got, char)
				}
			}
		})
	}
}

func TestSplitIntoChunks(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		chunkSize int
		expected  []string
	}{
		{
			name:      "short text",
			input:     "Hello world.",
			chunkSize: 10,
			expected:  []string{"Hello world."},
		},
		{
			name:      "split by sentence",
			input:     "This is the first sentence. This is the second sentence. This is the third.",
			chunkSize: 10, // ~40 characters limit
			expected: []string{
				"This is the first sentence.",
				"This is the second sentence.",
				"This is the third.",
			},
		},
		{
			name:      "forced split by space",
			input:     "A very long sentence without any punctuation that should eventually be split at a space if it exceeds the limit by too much.",
			chunkSize: 10, // ~40 characters limit
			// It splits at "A very long sentence without any" (at 32 chars)
			// Then remaining is "punctuation that should eventually be split at a space if it exceeds the limit by too much."
			// Next split is at "punctuation that should eventually be" (at 38 chars)
			// Then "split at a space if it exceeds the limit by too much."
			expected: []string{
				"A very long sentence without any",
				"punctuation that should eventually be",
				"split at a space if it exceeds the limit by too much.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitIntoChunks(tt.input, tt.chunkSize)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("splitIntoChunks() = %v, want %v", got, tt.expected)
			}
		})
	}
}
