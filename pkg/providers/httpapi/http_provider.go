// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package httpapi

import (
	"context"
	"time"

	"github.com/Kantemba/clawy/pkg/providers/openai_compat"
	openairesponses "github.com/Kantemba/clawy/pkg/providers/openai_responses"
)

type HTTPProvider struct {
	delegate          *openai_compat.Provider
	responsesDelegate *openairesponses.Provider
}

func NewHTTPProvider(apiKey, apiBase, proxy string) *HTTPProvider {
	return &HTTPProvider{
		delegate: openai_compat.NewProvider(apiKey, apiBase, proxy),
	}
}

func NewHTTPProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *HTTPProvider {
	return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(apiKey, apiBase, proxy, maxTokensField, "", 0, nil, nil)
}

func NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
	apiKey, apiBase, proxy, maxTokensField, userAgent string,
	requestTimeoutSeconds int,
	extraBody map[string]any,
	customHeaders map[string]string,
) *HTTPProvider {
	return &HTTPProvider{
		delegate: openai_compat.NewProvider(
			apiKey,
			apiBase,
			proxy,
			openai_compat.WithMaxTokensField(maxTokensField),
			openai_compat.WithRequestTimeout(time.Duration(requestTimeoutSeconds)*time.Second),
			openai_compat.WithExtraBody(extraBody),
			openai_compat.WithCustomHeaders(customHeaders),
			openai_compat.WithUserAgent(userAgent),
		),
	}
}

// NewOpenAIProviderWithMaxTokensFieldAndRequestTimeout creates a provider that
// uses Chat Completions for older OpenAI models and the Responses API for GPT-5.6.
// The latter is required for GPT-5.6 reasoning/tool-calling requests.
func NewOpenAIProviderWithMaxTokensFieldAndRequestTimeout(
	apiKey, apiBase, proxy, maxTokensField, userAgent string,
	requestTimeoutSeconds int,
	extraBody map[string]any,
	customHeaders map[string]string,
) *HTTPProvider {
	return &HTTPProvider{
		delegate: openai_compat.NewProvider(
			apiKey,
			apiBase,
			proxy,
			openai_compat.WithMaxTokensField(maxTokensField),
			openai_compat.WithRequestTimeout(time.Duration(requestTimeoutSeconds)*time.Second),
			openai_compat.WithExtraBody(extraBody),
			openai_compat.WithCustomHeaders(customHeaders),
			openai_compat.WithUserAgent(userAgent),
		),
		responsesDelegate: openairesponses.NewProvider(
			apiKey,
			apiBase,
			proxy,
			openairesponses.WithRequestTimeout(time.Duration(requestTimeoutSeconds)*time.Second),
			openairesponses.WithExtraBody(extraBody),
			openairesponses.WithCustomHeaders(customHeaders),
			openairesponses.WithUserAgent(userAgent),
		),
	}
}

func (p *HTTPProvider) useResponses(model string) bool {
	return p != nil && p.responsesDelegate != nil && openairesponses.IsGPT56Model(model)
}

func (p *HTTPProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	if p.useResponses(model) {
		return p.responsesDelegate.Chat(ctx, messages, tools, model, options)
	}
	return p.delegate.Chat(ctx, messages, tools, model, options)
}

// ChatStream implements providers.StreamingProvider by delegating to the
// OpenAI-compatible streaming endpoint (SSE with stream: true).
func (p *HTTPProvider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*LLMResponse, error) {
	if p.useResponses(model) {
		return p.responsesDelegate.ChatStream(ctx, messages, tools, model, options, onChunk)
	}
	return p.delegate.ChatStream(ctx, messages, tools, model, options, onChunk)
}

func (p *HTTPProvider) ChatStreamEvents(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(StreamChunk),
) (*LLMResponse, error) {
	if p.useResponses(model) {
		return p.responsesDelegate.ChatStreamEvents(ctx, messages, tools, model, options, onChunk)
	}
	return p.delegate.ChatStreamEvents(ctx, messages, tools, model, options, onChunk)
}

func (p *HTTPProvider) GetDefaultModel() string {
	return ""
}

func (p *HTTPProvider) SupportsNativeSearch() bool {
	if p != nil && p.responsesDelegate != nil {
		return p.responsesDelegate.SupportsNativeSearch()
	}
	return p.delegate.SupportsNativeSearch()
}

func (p *HTTPProvider) SupportsThinking() bool {
	if p == nil || p.delegate == nil {
		return false
	}
	if p.responsesDelegate != nil {
		return p.responsesDelegate.SupportsThinking()
	}
	return p.delegate.SupportsThinking()
}

func (p *HTTPProvider) SetProviderName(providerName string) {
	if p == nil || p.delegate == nil {
		return
	}
	p.delegate.SetProviderName(providerName)
}
