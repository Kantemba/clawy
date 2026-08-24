// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
)

func newTestChannel(t *testing.T, settings *config.WebhookSettings, allow ...string) (*WebhookChannel, *bus.MessageBus) {
	t.Helper()
	mb := bus.NewMessageBus()
	t.Cleanup(mb.Close)
	ch, err := NewWebhookChannel(
		&config.Channel{Type: config.ChannelWebHook, Enabled: true, AllowFrom: config.FlexibleStringSlice(allow)},
		config.ChannelWebHook,
		settings,
		mb,
	)
	if err != nil {
		t.Fatalf("NewWebhookChannel() error = %v", err)
	}
	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = ch.Stop(context.Background()) })
	return ch, mb
}

func TestSendBeforeStartReturnsErrNotRunning(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch, err := NewWebhookChannel(
		&config.Channel{Type: config.ChannelWebHook, Enabled: true},
		config.ChannelWebHook,
		&config.WebhookSettings{},
		mb,
	)
	if err != nil {
		t.Fatalf("NewWebhookChannel() error = %v", err)
	}

	_, err = ch.Send(context.Background(), bus.OutboundMessage{ChatID: "c1", Content: "hi"})
	if err == nil || err != channels.ErrNotRunning {
		t.Fatalf("Send() before Start = %v, want ErrNotRunning", err)
	}
}

func TestEffectivePath(t *testing.T) {
	cases := []struct {
		name     string
		channel  string
		override string
		want     string
	}{
		{"derived default", "hooks", "", "/webhook/hooks"},
		{"mixed case sanitized", "My Hook", "", "/webhook/my-hook"},
		{"override kept", "hooks", "/ingest", "/ingest"},
		{"override slash added", "hooks", "ingest", "/ingest"},
		{"override trailing slash trimmed", "hooks", "/ingest/", "/ingest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &config.WebhookSettings{Path: tc.override}
			if got := s.EffectivePath(tc.channel); got != tc.want {
				t.Errorf("EffectivePath(%q) with path %q = %q, want %q", tc.channel, tc.override, got, tc.want)
			}
		})
	}
}

func TestHandlePostPublishesInboundJSON(t *testing.T) {
	ch, mb := newTestChannel(t, &config.WebhookSettings{})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/webhook/webhook", "application/json",
		strings.NewReader(`{"message":"hello clawy","chat_id":"ci-alerts","sender_id":"ci-bot","display_name":"CI Bot"}`))
	if err != nil {
		t.Fatalf("POST error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}

	select {
	case msg := <-mb.InboundChan():
		if msg.Channel != config.ChannelWebHook {
			t.Errorf("Channel = %q, want %q", msg.Channel, config.ChannelWebHook)
		}
		if msg.ChatID != "ci-alerts" {
			t.Errorf("ChatID = %q, want ci-alerts", msg.ChatID)
		}
		if msg.Content != "hello clawy" {
			t.Errorf("Content = %q, want hello clawy", msg.Content)
		}
		if msg.Sender.PlatformID != "ci-bot" {
			t.Errorf("Sender.PlatformID = %q, want ci-bot", msg.Sender.PlatformID)
		}
		if msg.Sender.DisplayName != "CI Bot" {
			t.Errorf("Sender.DisplayName = %q, want CI Bot", msg.Sender.DisplayName)
		}
		if msg.Sender.CanonicalID != "webhook:ci-bot" {
			t.Errorf("Sender.CanonicalID = %q, want webhook:ci-bot", msg.Sender.CanonicalID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound message")
	}
}

func TestHandlePostRawTextAndDefaults(t *testing.T) {
	ch, mb := newTestChannel(t, &config.WebhookSettings{})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/webhook/webhook", strings.NewReader("deploy finished"))
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", resp.StatusCode)
	}

	var ack struct {
		OK     bool   `json:"ok"`
		ChatID string `json:"chat_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if !ack.OK || ack.ChatID != "default" {
		t.Fatalf("ack = %+v, want ok=true chat_id=default", ack)
	}

	select {
	case msg := <-mb.InboundChan():
		if msg.Content != "deploy finished" {
			t.Errorf("Content = %q, want deploy finished", msg.Content)
		}
		if msg.ChatID != "default" {
			t.Errorf("ChatID = %q, want default", msg.ChatID)
		}
		if msg.Sender.PlatformID != defaultSenderID {
			t.Errorf("Sender.PlatformID = %q, want %q", msg.Sender.PlatformID, defaultSenderID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound message")
	}
}

func TestHandlePostValidation(t *testing.T) {
	ch, _ := newTestChannel(t, &config.WebhookSettings{})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	cases := []struct {
		name        string
		method      string
		contentType string
		body        string
		wantStatus  int
	}{
		{"bad json", http.MethodPost, "application/json", `{not json`, http.StatusBadRequest},
		{"empty body", http.MethodPost, "text/plain", "   ", http.StatusBadRequest},
		{"missing message field", http.MethodPost, "application/json", `{"chat_id":"c1"}`, http.StatusBadRequest},
		{"blank message", http.MethodPost, "application/json", `{"message":"  "}`, http.StatusBadRequest},
		{"chat_id too long", http.MethodPost, "application/json", `{"message":"hi","chat_id":"` + strings.Repeat("x", 200) + `"}`, http.StatusBadRequest},
		{"method not allowed", http.MethodGet, "", "", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+"/webhook/webhook", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("NewRequest error = %v", err)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s error = %v", tc.method, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestTokenAuth(t *testing.T) {
	const token = "sekrit"
	ch, _ := newTestChannel(t, &config.WebhookSettings{Token: *config.NewSecureString(token)})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	post := func(headers map[string]string) int {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/webhook/webhook", strings.NewReader(`{"message":"hi"}`))
		if err != nil {
			t.Fatalf("NewRequest error = %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST error = %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if got := post(nil); got != http.StatusUnauthorized {
		t.Fatalf("POST without token = %d, want 401", got)
	}
	if got := post(map[string]string{"X-Clawy-Token": "nope"}); got != http.StatusUnauthorized {
		t.Fatalf("POST with wrong token = %d, want 401", got)
	}
	if got := post(map[string]string{"X-Clawy-Token": token}); got != http.StatusOK {
		t.Fatalf("POST with token header = %d, want 200", got)
	}
	if got := post(map[string]string{"Authorization": "Bearer " + token}); got != http.StatusOK {
		t.Fatalf("POST with bearer token = %d, want 200", got)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/webhook/webhook?token="+token, strings.NewReader(`{"message":"hi"}`))
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST with query token = %d, want 200", resp.StatusCode)
	}
}

func TestReplyURLForwarding(t *testing.T) {
	type received struct {
		Body  map[string]string
		Token string
	}

	got := make(chan received, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		got <- received{Body: body, Token: r.Header.Get("X-Clawy-Token")}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	const token = "reply-secret"
	ch, _ := newTestChannel(t, &config.WebhookSettings{
		Token:    *config.NewSecureString(token),
		ReplyURL: *config.NewSecureString(target.URL),
	})

	ids, err := ch.Send(context.Background(), bus.OutboundMessage{
		Channel:    config.ChannelWebHook,
		ChatID:     "ci-alerts",
		SessionKey: "webhook:ci-alerts",
		Content:    "build passed",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(ids) != 1 || ids[0] == "" {
		t.Fatalf("Send() ids = %v, want one non-empty id", ids)
	}

	select {
	case r := <-got:
		if r.Body["chat_id"] != "ci-alerts" {
			t.Errorf("reply chat_id = %q, want ci-alerts", r.Body["chat_id"])
		}
		if r.Body["content"] != "build passed" {
			t.Errorf("reply content = %q", r.Body["content"])
		}
		if r.Body["session_key"] != "webhook:ci-alerts" {
			t.Errorf("reply session_key = %q", r.Body["session_key"])
		}
		if r.Token != token {
			t.Errorf("reply token header = %q, want %q", r.Token, token)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for forwarded reply")
	}
}

func TestReplyDroppedWithoutURL(t *testing.T) {
	ch, _ := newTestChannel(t, &config.WebhookSettings{})
	ids, err := ch.Send(context.Background(), bus.OutboundMessage{
		Channel: config.ChannelWebHook,
		ChatID:  "c1",
		Content: "ignored reply",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(ids) != 1 || ids[0] == "" {
		t.Fatalf("Send() ids = %v, want one non-empty id", ids)
	}
}

func TestInvalidReplyURLRejected(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	_, err := NewWebhookChannel(
		&config.Channel{Type: config.ChannelWebHook, Enabled: true},
		config.ChannelWebHook,
		&config.WebhookSettings{ReplyURL: *config.NewSecureString("http://evil.example.com/hook")},
		mb,
	)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("NewWebhookChannel() error = %v, want HTTPS rejection", err)
	}
}
