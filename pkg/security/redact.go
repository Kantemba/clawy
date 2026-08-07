package security

import (
	"regexp"
	"strings"
	"sync"
)

// Redactor provides PII and secret redaction for logs and outputs.
type Redactor struct {
	patterns []*redactionPattern
	mu       sync.RWMutex
	cache    *strings.Replacer
	dirty    bool
}

type redactionPattern struct {
	regex *regexp.Regexp
	mask  string
}

// NewRedactor creates a new redactor with default patterns.
func NewRedactor() *Redactor {
	r := &Redactor{patterns: make([]*redactionPattern, 0)}
	r.addDefaultPatterns()
	return r
}

// WithCustomPattern adds a custom redaction pattern.
func (r *Redactor) WithCustomPattern(pattern, mask string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns = append(r.patterns, &redactionPattern{regex: re, mask: mask})
	r.dirty = true
	return nil
}

// Redact replaces sensitive information with [FILTERED].
func (r *Redactor) Redact(input string) string {
	if input == "" {
		return ""
	}
	r.mu.RLock()
	patterns := r.patterns
	r.mu.RUnlock()

	result := input
	for _, p := range patterns {
		result = p.regex.ReplaceAllString(result, p.mask)
	}
	return result
}

// RedactMap redacts all string values in a map.
func (r *Redactor) RedactMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = r.Redact(val)
		case map[string]any:
			result[k] = r.RedactMap(val)
		default:
			result[k] = v
		}
	}
	return result
}

// RedactBytes redacts sensitive data in byte slices.
func (r *Redactor) RedactBytes(b []byte) []byte {
	return []byte(r.Redact(string(b)))
}

func (r *Redactor) addDefaultPatterns() {
	patterns := []*redactionPattern{
		// JWT tokens (must come before generic API key patterns).
		{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`), "[JWT_FILTERED]"},
		// AWS access keys.
		{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[AWS_KEY_FILTERED]"},
		// AWS secret keys.
		{regexp.MustCompile(`\b[A-Za-z0-9/+=]{40}\b`), "[AWS_SECRET_FILTERED]"},
		// OpenAI API keys (sk-...).
		{regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`), "[OPENAI_KEY_FILTERED]"},
		// Anthropic API keys.
		{regexp.MustCompile(`\bant-[A-Za-z0-9\-]{20,}\b`), "[ANTHROPIC_KEY_FILTERED]"},
		// GitHub tokens.
		{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{36,}\b`), "[GITHUB_TOKEN_FILTERED]"},
		// Slack tokens.
		{regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9\-]{10,}\b`), "[SLACK_TOKEN_FILTERED]"},
		// Telegram bot tokens.
		{regexp.MustCompile(`\b\d{8,10}:[A-Za-z0-9_-]{30,50}\b`), "[TG_TOKEN_FILTERED]"},
		// Generic API keys (Bearer tokens) - after specific patterns.
		{regexp.MustCompile(`(?i)(?:bearer|api[_-]?key|api[_-]?secret|token)\s*[:=]\s*["']?[A-Za-z0-9_\-]{16,}["']?`), "[API_KEY_FILTERED]"},
		// Private keys.
		{regexp.MustCompile(`-----BEGIN [A-Z ]+PRIVATE KEY-----`), "[PRIVATE_KEY_FILTERED]"},
		// Email addresses.
		{regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`), "[EMAIL_FILTERED]"},
		// IPv4 addresses (for privacy).
		{regexp.MustCompile(`\b(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`), "[IP_FILTERED]"},
		// Credit card numbers (basic Luhn-format).
		{regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`), "[CARD_FILTERED]"},
		// Social security numbers (US).
		{regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), "[SSN_FILTERED]"},
		// Generic password patterns.
		{regexp.MustCompile(`(?i)(?:password|passwd|pwd)\s*[:=]\s*\S+`), "[PASSWORD_FILTERED]"},
	}
	r.patterns = patterns
}
