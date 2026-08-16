package openai_responses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderChatUsesResponsesSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Fatal("request must not contain legacy root reasoning_effort")
		}
		if _, ok := body["reasoning"]; ok {
			t.Fatal("request should omit reasoning when thinking_level is unset")
		}
		if _, ok := body["tools"]; !ok {
			t.Fatal("request should include function tools")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"object":"response",
			"status":"completed",
			"output":[{"type":"message","content":[{"type":"output_text","text":"Hello"}]}]
		}`))
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	response, err := p.Chat(
		testContext(t),
		[]Message{{Role: "user", Content: "hi"}},
		[]ToolDefinition{{Type: "function", Function: ToolFunctionDefinition{Name: "lookup"}}},
		"gpt-5.6-luna",
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Content != "Hello" {
		t.Fatalf("content = %q, want Hello", response.Content)
	}
}

func TestProviderMapsThinkingLevelToResponsesReasoning(t *testing.T) {
	p := NewProvider("key", "https://api.openai.com/v1", "")
	body, err := p.buildRequestBody(nil, nil, "gpt-5.6-luna", map[string]any{
		"thinking_level": "high",
	})
	if err != nil {
		t.Fatalf("buildRequestBody() error = %v", err)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("reasoning = %#v, want object", body["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Fatalf("reasoning.effort = %#v, want high", reasoning["effort"])
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatal("request must not contain legacy root reasoning_effort")
	}
}

func TestProviderOmitsTemperatureForGPT56(t *testing.T) {
	p := NewProvider("key", "https://api.openai.com/v1", "", WithExtraBody(map[string]any{
		"temperature": 0.2,
		"top_p":       0.5,
	}))
	body, err := p.buildRequestBody(nil, nil, "gpt-5.6-luna", map[string]any{
		"temperature": 0.7,
	})
	if err != nil {
		t.Fatalf("buildRequestBody() error = %v", err)
	}
	if _, ok := body["temperature"]; ok {
		t.Fatal("temperature must be omitted for GPT-5.6")
	}
	if _, ok := body["top_p"]; ok {
		t.Fatal("top_p must be omitted for GPT-5.6")
	}
}

func TestProviderTranslatesLegacyExtraBodyReasoningEffort(t *testing.T) {
	p := NewProvider("key", "https://api.openai.com/v1", "", WithExtraBody(map[string]any{
		"reasoning_effort": "low",
	}))
	body, err := p.buildRequestBody(nil, nil, "gpt-5.6-luna", nil)
	if err != nil {
		t.Fatalf("buildRequestBody() error = %v", err)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatal("legacy reasoning_effort must be removed")
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "low" {
		t.Fatalf("reasoning = %#v, want effort low", body["reasoning"])
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
