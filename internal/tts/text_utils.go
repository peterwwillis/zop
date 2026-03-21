package tts

import (
	"regexp"
	"strings"
)

func sanitizeMarkdown(text string) string {
	// 1. Remove code blocks entirely (TTS usually fails on them or reads them weirdly)
	text = regexp.MustCompile(`(?s)` + "```" + `.*?` + "```").ReplaceAllString(text, "")

	// 2. Handle links: [text](url) -> text
	text = regexp.MustCompile(`\[(.*?)\]\(.*?\)|<(.*?)>`).ReplaceAllStringFunc(text, func(s string) string {
		if strings.HasPrefix(s, "<") {
			return "" // Remove <url>
		}
		// Extract text from [text](url)
		re := regexp.MustCompile(`\[(.*?)\]\(.*?\)` )
		matches := re.FindStringSubmatch(s)
		if len(matches) > 1 {
			return matches[1]
		}
		return s
	})

	// 3. Remove other markdown formatting but keep the text
	text = regexp.MustCompile("`").ReplaceAllString(text, "")
	text = regexp.MustCompile(`(?m)^#{1,6}\s+`).ReplaceAllString(text, "")
	
	// Bold and Italic - simpler approach
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`__([^_]+)__`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(text, "$1")

	// Lists and blockquotes
	text = regexp.MustCompile(`(?m)^\s*[-*+]\s+\[[ xX]\]\s+`).ReplaceAllString(text, "  ") // Task lists
	text = regexp.MustCompile(`(?m)^\s*[-*+]\s+`).ReplaceAllString(text, "  ")
	text = regexp.MustCompile(`(?m)^\s*\d+\.\s+`).ReplaceAllString(text, "  ")
	text = regexp.MustCompile(`(?m)^\s*>\s+`).ReplaceAllString(text, "  ")
	
	// Horizontal rules
	text = regexp.MustCompile(`(?m)^\s*(\*\*\*+|---+|___+)\s*$`).ReplaceAllString(text, "")

	// Clean up multiple spaces and newlines
	text = regexp.MustCompile(` +`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

func splitIntoChunks(text string, chunkSize int) []string {
	if chunkSize <= 0 {
		return []string{text}
	}

	// Simple heuristic: 1 token ~= 4 characters
	charLimit := chunkSize * 4

	if len(text) <= charLimit {
		return []string{text}
	}

	var chunks []string

	// Split by sentences (approximately)
	// We look for punctuation followed by whitespace or EOF
	sentenceEnd := regexp.MustCompile(`([.!?])(\s+|$)`)
	
	remaining := text
	for len(remaining) > 0 {
		remaining = strings.TrimSpace(remaining)
		if remaining == "" {
			break
		}
		
		if len(remaining) <= charLimit {
			chunks = append(chunks, remaining)
			break
		}

		// Find the first sentence end within a reasonable range
		// If we can't find one within 1.5x charLimit, we force split at a space
		searchLimit := int(float64(charLimit) * 1.5)
		if searchLimit > len(remaining) {
			searchLimit = len(remaining)
		}

		locs := sentenceEnd.FindAllStringIndex(remaining[:searchLimit], -1)
		
		splitAt := -1
		if len(locs) > 0 {
			// Find the last sentence end that is within charLimit, or the first one if none are.
			for _, loc := range locs {
				if loc[1] <= charLimit {
					splitAt = loc[1]
				} else {
					if splitAt == -1 {
						splitAt = loc[1] // First one even if it exceeds charLimit
					}
					break
				}
			}
		}

		if splitAt == -1 {
			// No sentence end found, split at last space within charLimit
			lastSpace := strings.LastIndex(remaining[:charLimit], " ")
			if lastSpace == -1 {
				// No space found, hard split
				splitAt = charLimit
			} else {
				splitAt = lastSpace + 1 // Include the space
			}
		}

		chunks = append(chunks, strings.TrimSpace(remaining[:splitAt]))
		remaining = remaining[splitAt:]
	}

	return chunks
}
