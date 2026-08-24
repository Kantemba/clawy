// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

// Package webhook implements a generic inbound HTTP webhook channel served
// from Clawy's shared gateway HTTP server. External systems (home automation,
// CI pipelines, IoT devices, scripts) POST JSON or plain-text payloads and
// Clawy processes them like any other inbound chat message. Optionally, agent
// replies are forwarded to a configured reply URL via HTTP POST.
package webhook

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/identity"
	"github.com/Kantemba/clawy/pkg/logger"
)

const (
	// requestTimeout bounds inbound POST handling.
	requestTimeout = 30 * time.Second
	// replyClientTimeout bounds outbound reply forwarding.
	replyClientTimeout = 15 * time.Second
	// maxInboundBytes caps request body size to prevent memory exhaustion (DoS).
	maxInboundBytes = 1 << 20 // 1 MiB
	// maxChatIDLength caps caller-supplied chat identifiers.
	maxChatIDLength = 128
	// defaultChatID is used when the payload omits chat_id.
	defaultChatID = "default"
	// defaultSenderID is used when the payload omits sender_id.
	defaultSenderID = "webhook"
)

// WebhookChannel turns plain HTTP POSTs into agent conversations. Multiple
// instances are supported; each mounts its own path on the shared gateway
// server so distinct integrations get isolated sessions via distinct chat IDs.
type WebhookChannel struct {
	*channels.BaseChannel
	settings *config.WebhookSettings
	client   *http.Client
}

// NewWebhookChannel creates a new generic webhook channel. bc supplies
// allow_from and common fields; channelName is the config map key (used for
// the default mount path and sender identity); settings carries the
// webhook-specific options.
func NewWebhookChannel(
	bc *config.Channel,
	channelName string,
	settings *config.WebhookSettings,
	b *bus.MessageBus,
) (*WebhookChannel, error) {
	if settings == nil {
		settings = &config.WebhookSettings{}
	}
	if err := validateReplyURL(channelName, settings.ReplyURL.String()); err != nil {
		return nil, err
	}

	c := &WebhookChannel{
		BaseChannel: channels.NewBaseChannel(
			channelName,
			settings,
			b,
			bc.AllowFrom,
		),
		settings: settings,
		client:   &http.Client{Timeout: replyClientTimeout},
	}
	c.BaseChannel.SetOwner(c)
	return c, nil
}

// validateReplyURL enforces HTTPS everywhere except loopback hosts, where
// local integrations (Home Assistant, n8n, local scripts) commonly speak
// plain HTTP.
func validateReplyURL(channelName, raw string) error {
	replyURL := strings.TrimSpace(raw)
	if replyURL == "" {
		return nil
	}
	u, err := url.Parse(replyURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("webhook %q: invalid reply_url %q", channelName, replyURL)
	}
	if strings.EqualFold(u.Scheme, "https") {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return fmt.Errorf("webhook %q: reply_url must use HTTPS for non-loopback hosts (got %q)", channelName, u.Scheme)
	}
}

// Start implements Channel. The HTTP surface is mounted by the Manager via
// WebhookHandler, so Start only marks the channel ready.
func (c *WebhookChannel) Start(ctx context.Context) error {
	logger.InfoCF(c.Name(), "starting webhook channel", map[string]any{
		"path":      c.WebhookPath(),
		"has_reply": c.settings.ReplyURL.String() != "",
	})
	c.SetRunning(true)
	return nil
}

// Stop implements Channel.
func (c *WebhookChannel) Stop(_ context.Context) error {
	logger.InfoC(c.Name(), "Stopping webhook channel")
	c.SetRunning(false)
	return nil
}

// WebhookPath implements channels.WebhookHandler.
func (c *WebhookChannel) WebhookPath() string {
	return c.settings.EffectivePath(c.Name())
}

// ServeHTTP implements http.Handler for the shared gateway HTTP server.
func (c *WebhookChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.IsRunning() {
		http.Error(w, "Channel not running", http.StatusServiceUnavailable)
		return
	}
	if !c.authorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxInboundBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	content, chatID, senderID, displayName, err := parsePayload(body, r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	sender := bus.SenderInfo{
		Platform:    c.Name(),
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID(c.Name(), senderID),
		DisplayName: displayName,
	}
	inboundCtx := bus.InboundContext{
		Channel:   c.Name(),
		ChatID:    chatID,
		ChatType:  "direct",
		SenderID:  sender.CanonicalID,
		MessageID: uuid.New().String(),
	}
	if err := c.HandleMessageWithContext(ctx, chatID, content, nil, inboundCtx, sender); err != nil {
		logger.ErrorCF(c.Name(), "Failed to publish webhook message", map[string]any{
			"error":   err.Error(),
			"chat_id": chatID,
		})
		http.Error(w, "Failed to publish message", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "chat_id": chatID})
}

// authorized checks the optional shared token against the query parameter,
// the X-Clawy-Token header, or an Authorization: Bearer value (in that order).
func (c *WebhookChannel) authorized(r *http.Request) bool {
	token := c.settings.Token.String()
	if token == "" {
		return true
	}
	candidates := []string{
		r.URL.Query().Get("token"),
		r.Header.Get("X-Clawy-Token"),
	}
	if auth := r.Header.Get("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		candidates = append(candidates, strings.TrimSpace(auth[7:]))
	}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(cand), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

// webhookPayload is the liberal JSON shape accepted on POST. Any of
// message/text/content carries the prompt; the remaining fields are optional
// routing hints.
type webhookPayload struct {
	Message     string `json:"message"`
	Text        string `json:"text"`
	Content     string `json:"content"`
	ChatID      string `json:"chat_id"`
	SenderID    string `json:"sender_id"`
	DisplayName string `json:"display_name"`
}

// parsePayload normalizes a JSON object or raw text body into message content
// plus optional routing fields.
func parsePayload(body []byte, contentType string) (content, chatID, senderID, displayName string, err error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", "", "", "", errors.New("empty request body")
	}

	looksJSON := strings.Contains(strings.ToLower(contentType), "json") || trimmed[0] == '{'
	if looksJSON {
		var payload webhookPayload
		if jsonErr := json.Unmarshal(trimmed, &payload); jsonErr != nil {
			return "", "", "", "", errors.New("invalid JSON body")
		}
		content = strings.TrimSpace(payload.Message)
		if content == "" {
			content = strings.TrimSpace(payload.Text)
		}
		if content == "" {
			content = strings.TrimSpace(payload.Content)
		}
		if content == "" {
			return "", "", "", "", errors.New("missing message field (expected message, text, or content)")
		}
		chatID = strings.TrimSpace(payload.ChatID)
		senderID = strings.TrimSpace(payload.SenderID)
		displayName = strings.TrimSpace(payload.DisplayName)
	} else {
		content = string(trimmed)
	}

	if chatID == "" {
		chatID = defaultChatID
	}
	if len(chatID) > maxChatIDLength {
		return "", "", "", "", errors.New("chat_id too long")
	}
	if senderID == "" {
		senderID = defaultSenderID
	}
	return content, chatID, senderID, displayName, nil
}

// Send implements Channel — forwards agent replies to the configured reply
// URL as JSON. When no reply URL is configured the reply is dropped
// (fire-and-forget trigger integrations).
func (c *WebhookChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}
	content := strings.TrimSpace(msg.Content)
	msgID := uuid.New().String()
	if content == "" {
		return nil, nil
	}

	replyURL := strings.TrimSpace(c.settings.ReplyURL.String())
	if replyURL == "" {
		logger.DebugCF(c.Name(), "no reply_url configured; dropping reply", map[string]any{
			"chat_id": msg.ChatID,
		})
		return []string{msgID}, nil
	}

	payload, err := json.Marshal(map[string]string{
		"chat_id":     msg.ChatID,
		"content":     content,
		"session_key": msg.SessionKey,
	})
	if err != nil {
		return nil, fmt.Errorf("encode webhook reply: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, replyURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create webhook reply request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := c.settings.Token.String(); token != "" {
		req.Header.Set("X-Clawy-Token", token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, channels.ClassifyNetError(err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, channels.ClassifySendError(resp.StatusCode, fmt.Errorf("reply endpoint returned %s", resp.Status))
	}
	return []string{msgID}, nil
}