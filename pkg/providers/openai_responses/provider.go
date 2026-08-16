package openai_responses

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Kantemba/clawy/pkg/providers/common"
	orc "github.com/Kantemba/clawy/pkg/providers/openai_responses_common"
	"github.com/Kantemba/clawy/pkg/providers/protocoltypes"
)

type (
	LLMResponse            = protocoltypes.LLMResponse
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	StreamChunk            = protocoltypes.StreamChunk
)

const defaultStreamingReadIdleTimeout = 5 * time.Minute

// Provider implements the OpenAI Responses API for native OpenAI endpoints.
// It is intentionally separate from the generic OpenAI-compatible provider:
// Responses uses different input, tool, reasoning, and streaming schemas.
type Provider struct {
	apiKey        string
	apiBase       string
	httpClient    *http.Client
	extraBody     map[string]any
	customHeaders map[string]string
	userAgent     string
}

type Option func(*Provider)

func WithRequestTimeout(timeout time.Duration) Option {
	return func(p *Provider) {
		if timeout > 0 {
			p.httpClient.Timeout = timeout
		}
	}
}

func WithExtraBody(extraBody map[string]any) Option {
	return func(p *Provider) { p.extraBody = extraBody }
}

func WithCustomHeaders(customHeaders map[string]string) Option {
	return func(p *Provider) { p.customHeaders = customHeaders }
}

func WithUserAgent(userAgent string) Option {
	return func(p *Provider) { p.userAgent = userAgent }
}

func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:     apiKey,
		apiBase:    strings.TrimRight(apiBase, "/"),
		httpClient: common.NewHTTPClient(proxy),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// IsGPT56Model reports whether model belongs to the GPT-5.6 family.
func IsGPT56Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	model = strings.TrimPrefix(model, "openai/")
	return strings.HasPrefix(model, "gpt-5.6")
}

func (p *Provider) buildRequestBody(
	messages []Message, tools []ToolDefinition, model string, options map[string]any,
) (map[string]any, error) {
	input, instructions := orc.TranslateMessages(messages)
	model = strings.TrimPrefix(strings.TrimSpace(model), "openai/")
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Store: openai.Opt(false),
	}

	if instructions != "" {
		params.Instructions = openai.Opt(instructions)
	}
	if len(tools) > 0 {
		enableWebSearch, _ := options["native_search"].(bool)
		params.Tools = orc.TranslateTools(tools, enableWebSearch)
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
		}
	}
	if maxTokens, ok := common.AsInt(options["max_tokens"]); ok {
		params.MaxOutputTokens = openai.Opt(int64(maxTokens))
	}
	// GPT-5.6 reasoning models reject sampling temperature. The agent keeps a
	// general temperature option for other providers, so omit it only on this
	// model family rather than changing the global configuration semantics.
	if temperature, ok := common.AsFloat(options["temperature"]); ok && !IsGPT56Model(model) {
		params.Temperature = openai.Opt(temperature)
	}
	if cacheKey, ok := options["prompt_cache_key"].(string); ok && cacheKey != "" {
		params.PromptCacheKey = openai.Opt(cacheKey)
	}
	if effort, ok := reasoningEffort(options["thinking_level"]); ok {
		params.Reasoning.Effort = shared.ReasoningEffort(effort)
	}

	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Responses request: %w", err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		return nil, fmt.Errorf("failed to prepare Responses request: %w", err)
	}

	// Preserve the existing extra_body escape hatch, while translating the
	// legacy Chat Completions spelling so it cannot reach the Responses API.
	maps.Copy(body, p.extraBody)
	if IsGPT56Model(model) {
		// Also protect against a legacy/custom extra_body entry. GPT-5.6 does
		// not accept sampling controls such as temperature or top_p.
		delete(body, "temperature")
		delete(body, "top_p")
	}
	if legacy, ok := body["reasoning_effort"].(string); ok {
		delete(body, "reasoning_effort")
		if _, alreadySet := body["reasoning"]; !alreadySet {
			body["reasoning"] = map[string]any{"effort": legacy}
		}
	}

	return body, nil
}

func reasoningEffort(value any) (string, bool) {
	level, ok := value.(string)
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "off":
		return "none", true
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(level)), true
	default:
		// "adaptive" is a provider-specific setting and has no OpenAI
		// Responses equivalent. Omitting it preserves the model default.
		return "", false
	}
}

func (p *Provider) request(ctx context.Context, body map[string]any, stream bool) (*http.Response, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("OpenAI API base not configured")
	}
	if stream {
		body["stream"] = true
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.apiBase+"/responses",
		bytes.NewReader(encoded),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	if p.userAgent != "" {
		req.Header.Set("User-Agent", p.userAgent)
	}
	for key, value := range p.customHeaders {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}

	client := p.httpClient
	if stream {
		client = &http.Client{Transport: p.httpClient.Transport}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, common.HandleErrorResponse(resp, p.apiBase)
	}
	return resp, nil
}

func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	body, err := p.buildRequestBody(messages, tools, model, options)
	if err != nil {
		return nil, err
	}
	resp, err := p.request(ctx, body, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return orc.ParseResponseBody(resp.Body)
}

func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*LLMResponse, error) {
	return p.ChatStreamEvents(ctx, messages, tools, model, options, func(chunk StreamChunk) {
		if onChunk != nil && chunk.Content != "" {
			onChunk(chunk.Content)
		}
	})
}

func (p *Provider) ChatStreamEvents(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(StreamChunk),
) (*LLMResponse, error) {
	body, err := p.buildRequestBody(messages, tools, model, options)
	if err != nil {
		return nil, err
	}
	resp, err := p.request(ctx, body, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseStreamResponse(ctx, withStreamingReadIdleTimeout(resp.Body, defaultStreamingReadIdleTimeout), onChunk)
}

func parseStreamResponse(ctx context.Context, reader io.Reader, onChunk func(StreamChunk)) (*LLMResponse, error) {
	var textContent, reasoningContent strings.Builder
	var responseJSON json.RawMessage
	var eventName string
	var eventData strings.Builder

	processEvent := func(name, data string) error {
		if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
			return nil
		}
		switch name {
		case "response.output_text.delta":
			var event struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				return fmt.Errorf("failed to decode Responses text event: %w", err)
			}
			textContent.WriteString(event.Delta)
			if onChunk != nil {
				onChunk(StreamChunk{Content: textContent.String()})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.reasoning_content.delta":
			var event struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				return fmt.Errorf("failed to decode Responses reasoning event: %w", err)
			}
			reasoningContent.WriteString(event.Delta)
			if onChunk != nil {
				onChunk(StreamChunk{ReasoningContent: reasoningContent.String()})
			}
		case "response.completed", "response.incomplete", "response.failed", "response.cancelled":
			var event struct {
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				return fmt.Errorf("failed to decode Responses completion event: %w", err)
			}
			responseJSON = event.Response
		}
		return nil
	}

	flush := func() error {
		if eventData.Len() == 0 {
			return nil
		}
		err := processEvent(eventName, eventData.String())
		eventName = ""
		eventData.Reset()
		return err
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, ":"):
			continue
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if eventData.Len() > 0 {
				eventData.WriteByte('\n')
			}
			eventData.WriteString(data)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("streaming read error: %w", err)
	}
	if err := flush(); err != nil {
		return nil, err
	}

	if len(responseJSON) > 0 && string(responseJSON) != "null" {
		result, err := orc.ParseResponseBody(bytes.NewReader(responseJSON))
		if err != nil {
			return nil, err
		}
		if result.Content == "" {
			result.Content = textContent.String()
		}
		if result.ReasoningContent == "" {
			result.ReasoningContent = reasoningContent.String()
		}
		return result, nil
	}

	return &LLMResponse{
		Content:          textContent.String(),
		ReasoningContent: reasoningContent.String(),
		FinishReason:     "stop",
	}, nil
}

type streamingReadIdleTimeoutBody struct {
	body    io.ReadCloser
	timeout time.Duration
}

func withStreamingReadIdleTimeout(body io.ReadCloser, timeout time.Duration) io.ReadCloser {
	if body == nil || timeout <= 0 {
		return body
	}
	return &streamingReadIdleTimeoutBody{body: body, timeout: timeout}
}

func (b *streamingReadIdleTimeoutBody) Read(p []byte) (int, error) {
	timedOut := make(chan struct{})
	timer := time.AfterFunc(b.timeout, func() {
		close(timedOut)
		_ = b.body.Close()
	})
	n, err := b.body.Read(p)
	if !timer.Stop() {
		<-timedOut
		return n, fmt.Errorf("stream idle timeout after %s", b.timeout)
	}
	return n, err
}

func (b *streamingReadIdleTimeoutBody) Close() error { return b.body.Close() }

func (p *Provider) GetDefaultModel() string { return "" }

func (p *Provider) SupportsNativeSearch() bool {
	return isNativeOpenAIEndpoint(p.apiBase)
}

func (p *Provider) SupportsThinking() bool { return true }

func isNativeOpenAIEndpoint(apiBase string) bool {
	return strings.EqualFold(strings.TrimSpace(apiBase), "https://api.openai.com/v1")
}
