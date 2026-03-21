package tool

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/peterwwillis/zop/internal/config"
)

// PolicyChecker handles tool call validation against a policy.
type PolicyChecker struct {
	policy config.ToolPolicy
}

// NewPolicyChecker creates a new PolicyChecker.
func NewPolicyChecker(policy config.ToolPolicy) *PolicyChecker {
	return &PolicyChecker{policy: policy}
}

// IsAllowed checks if a tool call is allowed by the policy.
// toolName is the name of the tool being called.
// args is the JSON-encoded arguments string.
func (pc *PolicyChecker) IsAllowed(toolName string, args string) bool {
	if len(pc.policy.AllowList) == 0 {
		return false
	}

	// 1. Check DenyList (deny always trumps allow)
	for _, entry := range pc.policy.DenyList {
		if pc.matches(entry, toolName, args) {
			return false
		}
	}

	// 2. Check for DenyTags
	allowed := false
	for _, entry := range pc.policy.AllowList {
		if pc.matches(entry, toolName, args) {
			// If it matches an allow entry, we still check if it has a denied tag
			if pc.hasDeniedTag(entry.Tags) {
				return false
			}
			// If it matches and we have AllowTags, check if it has at least one allow tag
			if len(pc.policy.AllowTags) > 0 {
				if pc.hasAllowedTag(entry.Tags) {
					allowed = true
				}
			} else {
				allowed = true
			}
		}
	}

	return allowed
}

func (pc *PolicyChecker) matches(entry config.ToolEntry, toolName string, args string) bool {
	// If entry specifies a tool name, it must match.
	// If entry.Tool is empty, we assume it's for 'run_command' for backward compatibility.
	targetTool := entry.Tool
	if targetTool == "" {
		targetTool = "run_command"
	}
	if targetTool != toolName {
		return false
	}

	// If no filters are provided, the tool name match is sufficient.
	if len(entry.Exact) == 0 && entry.Regex == "" && len(entry.RegexArray) == 0 {
		return true
	}

	// Special handling for 'run_command' to match against the 'command' argument.
	if toolName == "run_command" {
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return false
		}
		command := input.Command
		parts := pc.detokenize(command)

		// Exact match (array-based)
		if len(entry.Exact) > 0 {
			if len(entry.Exact) == len(parts) {
				match := true
				for i, p := range parts {
					if p != entry.Exact[i] {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}

		// String-based regex
		if entry.Regex != "" {
			matched, _ := regexp.MatchString(entry.Regex, command)
			if matched {
				return true
			}
		}

		// Array-based regex
		if len(entry.RegexArray) > 0 {
			if len(entry.RegexArray) <= len(parts) {
				match := true
				for i, pattern := range entry.RegexArray {
					matched, _ := regexp.MatchString(pattern, parts[i])
					if !matched {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
		return false
	}

	// For other tools (MCP), match filters against the raw JSON arguments string.
	if entry.Regex != "" {
		matched, _ := regexp.MatchString(entry.Regex, args)
		if matched {
			return true
		}
	}
	// Exact and RegexArray don't have a clear mapping for arbitrary JSON objects yet,
	// so for now we only support Regex for MCP argument filtering.
	// Matching against the whole JSON string is quite flexible though.

	return false
}

func (pc *PolicyChecker) hasDeniedTag(tags []string) bool {
	for _, t := range tags {
		for _, dt := range pc.policy.DenyTags {
			if t == dt {
				return true
			}
		}
	}
	return false
}

func (pc *PolicyChecker) hasAllowedTag(tags []string) bool {
	for _, t := range tags {
		for _, at := range pc.policy.AllowTags {
			if t == at {
				return true
			}
		}
	}
	return false
}

// detokenize handles simple shell-like splitting (quotes, spaces).
func (pc *PolicyChecker) detokenize(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case (r == '"' || r == '\'') && !inQuote:
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
		case !inQuote && r == ' ':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
