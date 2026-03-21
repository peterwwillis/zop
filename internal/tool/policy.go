package tool

import (
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

// IsAllowed checks if a command is allowed by the policy.
// It returns true if allowed, false if denied.
// By default, no tools are allowed unless AllowList is populated and matches.
func (pc *PolicyChecker) IsAllowed(command string) bool {
	if len(pc.policy.AllowList) == 0 {
		return false
	}

	parts := pc.detokenize(command)

	// 1. Check DenyList (deny always trumps allow)
	for _, entry := range pc.policy.DenyList {
		if pc.matches(entry, command, parts) {
			return false
		}
	}

	// 2. Check for DenyTags
	// Check if ANY matching entry in AllowList or DenyList has a denied tag.
	// User said "deny must always trump allow".

	allowed := false
	for _, entry := range pc.policy.AllowList {
		if pc.matches(entry, command, parts) {
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

func (pc *PolicyChecker) matches(entry config.ToolEntry, command string, parts []string) bool {
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
