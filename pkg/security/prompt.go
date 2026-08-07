package security

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// PromptInjectionDefense provides layered protection against prompt injection attacks.
type PromptInjectionDefense struct {
	maxToolResultLength int
	escapeToolResults   bool
	allowedToolNames    map[string]struct{}
}

// NewPromptInjectionDefense creates a defense configuration.
func NewPromptInjectionDefense() *PromptInjectionDefense {
	return &PromptInjectionDefense{
		maxToolResultLength: 50000,
		escapeToolResults:   true,
		allowedToolNames:    make(map[string]struct{}),
	}
}

// WithMaxToolResultLength sets the maximum allowed tool result length.
func (d *PromptInjectionDefense) WithMaxToolResultLength(max int) *PromptInjectionDefense {
	d.maxToolResultLength = max
	return d
}

// WithEscapeToolResults enables escaping of tool results before injection.
func (d *PromptInjectionDefense) WithEscapeToolResults(escape bool) *PromptInjectionDefense {
	d.escapeToolResults = escape
	return d
}

// WithAllowedToolNames restricts which tools the LLM can invoke.
func (d *PromptInjectionDefense) WithAllowedToolNames(names ...string) *PromptInjectionDefense {
	for _, name := range names {
		d.allowedToolNames[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return d
}

// injectionPatterns matches common prompt injection attempts.
var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above|earlier)\s+(instructions?|prompts?|commands?)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a\s+)?(different|new|another|helpful)`),
	regexp.MustCompile(`(?i)(system|developer|root|admin)\s*:\s*`),
	regexp.MustCompile(`(?i)<\s*(system|instructions?|prompt)\s*>`),
	regexp.MustCompile(`(?i)(?:\[)(system|instructions?|developer)(?:\])`),
	regexp.MustCompile(`(?i)(?:^|\n)\s*[-]{3,}\s*(?:system|instructions?)`),
	regexp.MustCompile(`(?i)disregard\s+(your\s+)?(programming|training|instructions?)`),
	regexp.MustCompile(`(?i)new\s+(persona|identity|role|behavior)\s*[:：]`),
	regexp.MustCompile(`(?i)DAN\s*[（(]?\s*do\s+anything\s+now`),
	regexp.MustCompile(`(?i)(?:^|\n)\s*(?:` + "```" + `)\s*(?:system|instructions?)`),
}

// DetectInjection checks if content contains prompt injection patterns.
// Returns true if injection is detected.
func (d *PromptInjectionDefense) DetectInjection(content string) bool {
	if content == "" {
		return false
	}
	for _, pattern := range injectionPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

// SanitizeToolResult prepares a tool result for safe injection into LLM context.
// It truncates, escapes, and wraps the result to prevent injection.
func (d *PromptInjectionDefense) SanitizeToolResult(toolName, result string) string {
	if result == "" {
		return ""
	}

	if len(result) > d.maxToolResultLength {
		result = result[:d.maxToolResultLength] + "\n[TRUNCATED: result exceeded maximum length]"
	}

	if d.escapeToolResults {
		result = escapeForLLMContext(result)
	}

	return result
}

// ValidateToolCall checks if a tool invocation is allowed.
func (d *PromptInjectionDefense) ValidateToolCall(toolName string, args map[string]any) error {
	if len(d.allowedToolNames) > 0 {
		if _, ok := d.allowedToolNames[strings.ToLower(strings.TrimSpace(toolName))]; !ok {
			return &SecurityError{
				Category: "tool_policy",
				Message:  "tool '" + toolName + "' is not in the allowed tools list",
			}
		}
	}

	// Check for injection in string arguments.
	for key, val := range args {
		if str, ok := val.(string); ok {
			if d.DetectInjection(str) {
				return &SecurityError{
					Category: "prompt_injection",
					Message:  "potential prompt injection detected in argument '" + key + "'",
				}
			}
		}
	}

	return nil
}

// ValidateJSONOutput attempts to extract and validate JSON from LLM output,
// preventing injection through malformed tool call responses.
func ValidateJSONOutput(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &SecurityError{Category: "parse", Message: "empty output"}
	}

	if !utf8.ValidString(raw) {
		return &SecurityError{Category: "encoding", Message: "invalid UTF-8 in output"}
	}

	// Strip markdown code fences if present.
	raw = stripCodeFences(raw)

	// Find the first valid JSON object/array in the output.
	jsonStart, jsonEnd := findJSONBounds(raw)
	if jsonStart < 0 || jsonEnd <= jsonStart {
		return &SecurityError{Category: "parse", Message: "no valid JSON found in output"}
	}

	jsonStr := raw[jsonStart : jsonEnd+1]
	if err := json.Unmarshal([]byte(jsonStr), target); err != nil {
		return &SecurityError{Category: "parse", Message: "JSON parse error: " + err.Error()}
	}

	return nil
}

// SecurityError represents a security policy violation.
type SecurityError struct {
	Category string
	Message  string
}

func (e *SecurityError) Error() string {
	return "security [" + e.Category + "]: " + e.Message
}

// escapeForLLMContext wraps tool output to prevent the LLM from interpreting
// it as instructions. Uses unambiguous delimiters.
func escapeForLLMContext(s string) string {
	// Replace delimiter-like sequences that could break out of wrapping.
	s = strings.ReplaceAll(s, "<<<TOOL_RESULT_END>>>", "<<<TOOL_RESULT_END_ESCAPED>>>")
	return s
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if newline := strings.Index(s[3:], "\n"); newline >= 0 {
			s = s[3+newline+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

func findJSONBounds(s string) (start, end int) {
	start = -1
	depth := 0
	inString := false
	escape := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}

		switch ch {
		case '{', '[':
			if depth == 0 {
				start = i
			}
			depth++
		case '}', ']':
			depth--
			if depth == 0 && start >= 0 {
				return start, i
			}
		}
	}
	return -1, -1
}
