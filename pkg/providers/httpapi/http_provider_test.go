package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProviderRoutesGPT56ToResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "gpt-5.6-luna" {
			t.Fatalf("model = %#v, want gpt-5.6-luna", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"object":"response",
			"status":"completed",
			"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]
		}`))
	}))
	defer server.Close()

	p := NewOpenAIProviderWithMaxTokensFieldAndRequestTimeout(
		"key", server.URL, "", "", "", 0, nil, nil,
	)
	response, err := p.Chat(
		context.Background(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		"gpt-5.6-luna",
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Content != "ok" {
		t.Fatalf("content = %q, want ok", response.Content)
	}
}
