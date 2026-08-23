// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package webchat

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
)

func newTestChannel(t *testing.T, settings *config.WebChatSettings, allow ...string) (*WebChatChannel, *bus.MessageBus) {
	t.Helper()
	mb := bus.NewMessageBus()
	t.Cleanup(mb.Close)
	ch, err := NewWebChatChannel(
		&config.Channel{Type: config.ChannelWebChat, Enabled: true, AllowFrom: config.FlexibleStringSlice(allow)},
		settings,
		mb,
	)
	if err != nil {
		t.Fatalf("NewWebChatChannel() error = %v", err)
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

	ch, err := NewWebChatChannel(
		&config.Channel{Type: config.ChannelWebChat, Enabled: true},
		&config.WebChatSettings{},
		mb,
	)
	if err != nil {
		t.Fatalf("NewWebChatChannel() error = %v", err)
	}

	_, err = ch.Send(context.Background(), bus.OutboundMessage{ChatID: "c1", Content: "hi"})
	if err == nil || err != channels.ErrNotRunning {
		t.Fatalf("Send() before Start = %v, want ErrNotRunning", err)
	}
}

func TestHandleSendPublishesInbound(t *testing.T) {
	ch, mb := newTestChannel(t, &config.WebChatSettings{})

	srv := httptest.NewServer(ch)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/webchat/send", "application/json",
		strings.NewReader(`{"id":"chat-1","message":"hello clawy"}`))
	if err != nil {
		t.Fatalf("POST /send error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /send status = %d, want 200", resp.StatusCode)
	}

	select {
	case msg := <-mb.InboundChan():
		if msg.Channel != config.ChannelWebChat {
			t.Errorf("Channel = %q, want %q", msg.Channel, config.ChannelWebChat)
		}
		if msg.ChatID != "chat-1" {
			t.Errorf("ChatID = %q, want chat-1", msg.ChatID)
		}
		if msg.Content != "hello clawy" {
			t.Errorf("Content = %q, want %q", msg.Content, "hello clawy")
		}
		if msg.Sender.PlatformID != "chat-1" {
			t.Errorf("Sender.PlatformID = %q, want chat-1", msg.Sender.PlatformID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound message")
	}
}

func TestHandleSendValidation(t *testing.T) {
	ch, _ := newTestChannel(t, &config.WebChatSettings{})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	cases := []struct {
		name string
		body string
	}{
		{"bad json", `{not json`},
		{"missing id", `{"message":"hi"}`},
		{"missing message", `{"id":"c1"}`},
		{"blank message", `{"id":"c1","message":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/webchat/send", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST /send error = %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestHandleSendNotAllowedIsDropped(t *testing.T) {
	ch, mb := newTestChannel(t, &config.WebChatSettings{}, "someone-else")
	srv := httptest.NewServer(ch)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/webchat/send", "application/json",
		strings.NewReader(`{"id":"intruder","message":"let me in"}`))
	if err != nil {
		t.Fatalf("POST /send error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (drop is silent)", resp.StatusCode)
	}

	select {
	case msg := <-mb.InboundChan():
		t.Fatalf("unexpected inbound message for %+v", msg.Context)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestSSEBroadcastReachesClient(t *testing.T) {
	ch, _ := newTestChannel(t, &config.WebChatSettings{})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/webchat/events?id=c1", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events error = %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)
	connected := false
	for !connected {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading connected marker: %v", err)
		}
		if strings.HasPrefix(line, ": connected") {
			connected = true
		}
	}

	if _, err := ch.Send(context.Background(), bus.OutboundMessage{
		Channel: config.ChannelWebChat,
		ChatID:  "c1",
		Content: "hi there",
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading event: %v", err)
		}
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "hi there") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for broadcast event")
		default:
		}
	}
}
func TestTokenAuth(t *testing.T) {
	const token = "sekrit"
	ch, mb := newTestChannel(t, &config.WebChatSettings{Token: *config.NewSecureString(token)})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	post := func(headers map[string]string) int {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/webchat/send", strings.NewReader(`{"id":"c1","message":"hi"}`))
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

	// No token → unauthorized.
	if got := post(nil); got != http.StatusUnauthorized {
		t.Fatalf("/send without token = %d, want 401", got)
	}
	// Wrong token → unauthorized.
	if got := post(map[string]string{"X-Clawy-Token": "nope"}); got != http.StatusUnauthorized {
		t.Fatalf("/send with wrong token = %d, want 401", got)
	}
	// Correct header token → accepted.
	if got := post(map[string]string{"X-Clawy-Token": token}); got != http.StatusOK {
		t.Fatalf("/send with token header = %d, want 200", got)
	}

	// Events stream requires the token too.
	resp, err := http.Get(srv.URL + "/webchat/events?id=c1")
	if err != nil {
		t.Fatalf("GET /events error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/events without token = %d, want 401", resp.StatusCode)
	}

	// Query-token variant connects.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/webchat/events?id=c1&token="+token, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext error = %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events?token error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/events with query token = %d, want 200", resp.StatusCode)
	}

	// The UI page itself stays reachable without a token.
	pageResp, err := http.Get(srv.URL + "/webchat/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	defer pageResp.Body.Close()
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("/ status = %d, want 200", pageResp.StatusCode)
	}

	select {
	case <-mb.InboundChan():
	case <-time.After(200 * time.Millisecond):
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	ch, _ := newTestChannel(t, &config.WebChatSettings{})
	srv := httptest.NewServer(ch)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/webchat/nope")
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}