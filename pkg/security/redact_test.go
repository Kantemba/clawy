package security

import (
	"strings"
	"testing"
)

func TestRedactor(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"AWS key", "key=AKIAIOSFODNN7EXAMPLE", "AWS_KEY_FILTERED"},
		{"OpenAI key", "using sk-1234567890abcdefghijklmnop", "OPENAI_KEY_FILTERED"},
		{"Bearer token", "Bearer: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", "JWT_FILTERED"},
		{"email", "Contact user@example.com for info", "EMAIL_FILTERED"},
		{"password", "password=mysecret123", "PASSWORD_FILTERED"},
		{"Telegram token", "123456789:ABCdefGHIjklMNOpqrsTUVwxyz1234567890", "TG_TOKEN_FILTERED"},
		{"clean text", "Hello world", "Hello world"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----", "PRIVATE_KEY_FILTERED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Redact(%q) = %q, should contain %q", tt.input, result, tt.contains)
			}
		})
	}
}

func TestRedactMap(t *testing.T) {
	r := NewRedactor()
	input := map[string]any{
		"api_key": "sk-1234567890abcdefghijklmnop",
		"name":    "test",
		"nested": map[string]any{
			"password": "secret123",
		},
	}
	result := r.RedactMap(input)
	if !strings.Contains(result["api_key"].(string), "FILTERED") {
		t.Error("api_key should be filtered")
	}
	if result["name"] != "test" {
		t.Error("name should not be filtered")
	}
}

func TestCustomPattern(t *testing.T) {
	r := NewRedactor()
	err := r.WithCustomPattern(`secret_\w+`, "[CUSTOM_FILTERED]")
	if err != nil {
		t.Fatalf("WithCustomPattern failed: %v", err)
	}
	result := r.Redact("my_secret_token is here")
	if !strings.Contains(result, "CUSTOM_FILTERED") {
		t.Errorf("expected custom pattern to match, got %s", result)
	}
}
