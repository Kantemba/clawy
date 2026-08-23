// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

// Package webchat implements an OpenClaw-style browser chat surface served
// from Clawy's shared gateway HTTP server. Browsers open the mount path,
// receive replies over Server-Sent Events, and post messages back via JSON.
package webchat

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/identity"
	"github.com/Kantemba/clawy/pkg/logger"
)

const (
	// sseBuffer caps events queued per connected browser before drops occur.
	sseBuffer = 32
	// requestTimeout bounds inbound POST handling.
	requestTimeout = 30 * time.Second
	// maxInboundBytes caps the size of a single posted chat message.
	maxInboundBytes = 64 * 1024
)

// webchatClient is one connected browser tab.
type webchatClient struct {
	id     string
	events chan string
	done   chan struct{}
}

// WebChatChannel serves a lightweight chat UI on the gateway HTTP server.
type WebChatChannel struct {
	*channels.BaseChannel
	settings *config.WebChatSettings

	mu      sync.RWMutex
	clients map[string]map[*webchatClient]struct{}

	ctx    context.Context
	cancel context.CancelFunc
}

// NewWebChatChannel creates the channel. bc supplies allow_from and common
// fields; settings carries the webchat-specific options.
func NewWebChatChannel(
	bc *config.Channel,
	settings *config.WebChatSettings,
	b *bus.MessageBus,
) (*WebChatChannel, error) {
	c := &WebChatChannel{
		BaseChannel: channels.NewBaseChannel(
			config.ChannelWebChat,
			settings,
			b,
			bc.AllowFrom,
			channels.WithMaxMessageLength(settings.EffectiveMaxMessageLength()),
		),
		settings: settings,
		clients:  make(map[string]map[*webchatClient]struct{}),
	}
	c.BaseChannel.SetOwner(c)
	return c, nil
}

// Start implements Channel. The HTTP surface is mounted by the Manager via
// WebhookHandler, so Start only marks the channel ready.
func (c *WebChatChannel) Start(ctx context.Context) error {
	logger.InfoCF(config.ChannelWebChat, "starting webchat channel", map[string]any{
		"path": c.WebhookPath(),
	})
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.SetRunning(true)
	return nil
}

// Stop implements Channel.
func (c *WebChatChannel) Stop(_ context.Context) error {
	logger.InfoC(config.ChannelWebChat, "Stopping WebChat channel")
	c.SetRunning(false)

	c.mu.Lock()
	clients := make([]*webchatClient, 0, 8)
	for _, perChat := range c.clients {
		for cl := range perChat {
			clients = append(clients, cl)
		}
	}
	c.clients = make(map[string]map[*webchatClient]struct{})
	c.mu.Unlock()

	for _, cl := range clients {
		close(cl.done)
	}
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// WebhookPath implements channels.WebhookHandler.
func (c *WebChatChannel) WebhookPath() string { return c.settings.EffectivePath() }

// ServeHTTP implements http.Handler for the shared HTTP server.
func (c *WebChatChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mount := strings.TrimSuffix(c.WebhookPath(), "/")
	path := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), mount)

	switch path {
	case "", "/":
		// The UI page itself is safe to serve; it prompts for a token
		// before opening the event stream when one is configured.
		c.servePage(w, r)
	case "/events":
		c.handleEvents(w, r)
	case "/send":
		c.handleSend(w, r)
	default:
		http.NotFound(w, r)
	}
}

// authorized checks the optional shared token against the query parameter,
// the X-Clawy-Token header, or the remembered cookie (in that order).
func (c *WebChatChannel) authorized(r *http.Request) bool {
	token := c.settings.Token.String()
	if token == "" {
		return true
	}
	candidates := []string{
		r.URL.Query().Get("token"),
		r.Header.Get("X-Clawy-Token"),
	}
	if ck, err := r.Cookie("clawy_webchat_token"); err == nil {
		candidates = append(candidates, ck.Value)
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

// handleEvents streams outbound assistant messages to one browser as SSE.
func (c *WebChatChannel) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.authorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	chatID := strings.TrimSpace(r.URL.Query().Get("id"))
	if chatID == "" || len(chatID) > 128 {
		http.Error(w, "Missing or invalid chat id", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	cl := &webchatClient{
		id:     chatID,
		events: make(chan string, sseBuffer),
		done:   make(chan struct{}),
	}
	c.mu.Lock()
	if c.clients[chatID] == nil {
		c.clients[chatID] = make(map[*webchatClient]struct{})
	}
	c.clients[chatID][cl] = struct{}{}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if perChat, ok := c.clients[chatID]; ok {
			delete(perChat, cl)
			if len(perChat) == 0 {
				delete(c.clients, chatID)
			}
		}
		c.mu.Unlock()
	}()

	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.ctx.Done():
			return
		case <-cl.done:
			return
		case evt := <-cl.events:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", evt); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// handleSend accepts a posted chat message and publishes it to the bus.
func (c *WebChatChannel) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	var payload struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	chatID := strings.TrimSpace(payload.ID)
	content := strings.TrimSpace(payload.Message)
	if chatID == "" || content == "" {
		http.Error(w, "Both id and message are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	sender := bus.SenderInfo{
		Platform:    config.ChannelWebChat,
		PlatformID:  chatID,
		CanonicalID: identity.BuildCanonicalID(config.ChannelWebChat, chatID),
	}
	inboundCtx := bus.InboundContext{
		Channel:   config.ChannelWebChat,
		ChatID:    chatID,
		ChatType:  "direct",
		SenderID:  sender.CanonicalID,
		MessageID: uuid.New().String(),
	}
	if err := c.HandleMessageWithContext(ctx, chatID, content, nil, inboundCtx, sender); err != nil {
		logger.ErrorCF(config.ChannelWebChat, "Failed to publish webchat message", map[string]any{
			"error": err.Error(),
		})
		http.Error(w, "Failed to publish message", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// Send implements Channel — broadcasts the reply to every connected browser.
func (c *WebChatChannel) Send(_ context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil, nil
	}

	evt, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return nil, fmt.Errorf("encode webchat event: %w", err)
	}
	msgID := uuid.New().String()

	c.mu.RLock()
	targets := make([]*webchatClient, 0, 2)
	for cl := range c.clients[msg.ChatID] {
		targets = append(targets, cl)
	}
	c.mu.RUnlock()

	for _, cl := range targets {
		select {
		case cl.events <- string(evt):
		default:
			// Browser too slow; drop instead of blocking other receivers.
			logger.DebugCF(config.ChannelWebChat, "dropped event for slow client", map[string]any{
				"chat_id": msg.ChatID,
			})
		}
	}
	return []string{msgID}, nil
}
