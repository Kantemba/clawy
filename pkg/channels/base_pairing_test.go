// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package channels

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/pairing"
)

// recordingOwner stands in for the concrete channel embedding BaseChannel so
// pairing notices dispatched via owner.Send can be observed in tests.
type recordingOwner struct {
	*BaseChannel
	sent []bus.OutboundMessage
}

func (o *recordingOwner) Name() string { return "test" }
func (o *recordingOwner) Start(context.Context) error {
	o.SetRunning(true)
	return nil
}
func (o *recordingOwner) Stop(context.Context) error {
	o.SetRunning(false)
	return nil
}
func (o *recordingOwner) Send(_ context.Context, msg bus.OutboundMessage) ([]string, error) {
	o.sent = append(o.sent, msg)
	return []string{"pairing-notice"}, nil
}
func (o *recordingOwner) ReasoningChannelID() string { return "" }

func newPairingTestChannel(t *testing.T, allowList []string) (*BaseChannel, *recordingOwner, *bus.MessageBus) {
	t.Helper()
	msgBus := bus.NewMessageBus()
	t.Cleanup(msgBus.Close)
	base := NewBaseChannel("test", nil, msgBus, allowList)
	owner := &recordingOwner{BaseChannel: base}
	base.SetOwner(owner)
	base.SetPairingManager(pairing.NewManager(t.TempDir()))
	return base, owner, msgBus
}

func TestHandleMessage_PairingNoticeForUnknownDMSender(t *testing.T) {
	base, owner, _ := newPairingTestChannel(t, []string{"owner-id"})

	err := base.HandleMessageWithContext(context.Background(), "chat-1", "hello", nil, bus.InboundContext{
		Channel:  "test",
		ChatID:   "chat-1",
		SenderID: "stranger",
		ChatType: "direct",
	}, bus.SenderInfo{Platform: "test", PlatformID: "stranger", CanonicalID: "test:stranger"})
	if err != nil {
		t.Fatalf("HandleMessageWithContext() error = %v", err)
	}
	if len(owner.sent) != 1 {
		t.Fatalf("got %d pairing notices, want 1", len(owner.sent))
	}
	msg := owner.sent[0]
	if msg.ChatID != "chat-1" || msg.Content == "" {
		t.Fatalf("unexpected notice %+v", msg)
	}

	// Extract the 6-digit code from the notice and approve it through the
	// same store the channel uses — this is the exact CLI round-trip.
	codeRe := regexp.MustCompile(`clawy pairing approve test (\d{6})`)
	m := codeRe.FindStringSubmatch(msg.Content)
	if m == nil {
		t.Fatalf("notice %q missing approve command with 6-digit code", msg.Content)
	}
	req, err := base.GetPairingManager().Approve("test", m[1])
	if err != nil {
		t.Fatalf("Approve(%s) error = %v", m[1], err)
	}
	if req.SenderID != "test:stranger" {
		t.Errorf("approved sender = %q, want test:stranger", req.SenderID)
	}
}

func TestHandleMessage_PairingNoticeRateLimited(t *testing.T) {
	base, owner, _ := newPairingTestChannel(t, []string{"owner-id"})
	in := bus.InboundContext{
		Channel: "test", ChatID: "chat-1", SenderID: "stranger", ChatType: "direct",
	}
	sender := bus.SenderInfo{Platform: "test", PlatformID: "stranger"}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := base.HandleMessageWithContext(ctx, "chat-1", "hi", nil, in, sender); err != nil {
			t.Fatalf("message %d: HandleMessageWithContext() error = %v", i+1, err)
		}
	}
	// One notice for the fresh request; repeats within the cooldown are silent.
	if len(owner.sent) != 1 {
		t.Fatalf("got %d notices, want exactly 1 (rate limited)", len(owner.sent))
	}
}

func TestHandleMessage_NoPairingInGroupChats(t *testing.T) {
	base, owner, _ := newPairingTestChannel(t, []string{"owner-id"})

	err := base.HandleMessageWithContext(context.Background(), "group-1", "hello", nil, bus.InboundContext{
		Channel:  "test",
		ChatID:   "group-1",
		SenderID: "stranger",
		ChatType: "group",
	}, bus.SenderInfo{Platform: "test", PlatformID: "stranger", CanonicalID: "test:stranger"})
	if err != nil {
		t.Fatalf("HandleMessageWithContext() error = %v", err)
	}
	if len(owner.sent) != 0 {
		t.Fatalf("group chat must not trigger pairing, got %d notices", len(owner.sent))
	}
}

func TestHandleMessage_AllowedSenderNeverPaired(t *testing.T) {
	base, owner, _ := newPairingTestChannel(t, []string{"known-id"})

	err := base.HandleMessageWithContext(context.Background(), "chat-1", "hello", nil, bus.InboundContext{
		Channel:  "test",
		ChatID:   "chat-1",
		SenderID: "known-id",
		ChatType: "direct",
	}, bus.SenderInfo{Platform: "test", PlatformID: "known-id", CanonicalID: "test:known-id"})
	if err != nil {
		t.Fatalf("HandleMessageWithContext() error = %v", err)
	}
	if len(owner.sent) != 0 {
		t.Fatalf("allowed sender must not receive a pairing notice, got %d", len(owner.sent))
	}
}

func TestHandleMessage_ApprovedSenderAdmittedWithoutAllowList(t *testing.T) {
	base, owner, msgBus := newPairingTestChannel(t, []string{"owner-id"})
	ctx := context.Background()
	in := bus.InboundContext{
		Channel: "test", ChatID: "chat-1", SenderID: "stranger", ChatType: "direct",
	}
	sender := bus.SenderInfo{Platform: "test", PlatformID: "stranger", CanonicalID: "test:stranger"}

	// First contact: pairing notice with the code.
	if err := base.HandleMessageWithContext(ctx, "chat-1", "hello", nil, in, sender); err != nil {
		t.Fatalf("first message: HandleMessageWithContext() error = %v", err)
	}
	codeRe := regexp.MustCompile(`clawy pairing approve test (\d{6})`)
	m := codeRe.FindStringSubmatch(owner.sent[0].Content)
	if m == nil {
		t.Fatalf("notice %q missing code", owner.sent[0].Content)
	}

	// Owner approves via the CLI flow (same store the channel reads).
	if _, err := base.GetPairingManager().Approve("test", m[1]); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	noticeCount := len(owner.sent)

	// Next message from the approved sender must be admitted onto the bus
	// without any further pairing notice.
	if err := base.HandleMessageWithContext(ctx, "chat-1", "are you there?", nil, in, sender); err != nil {
		t.Fatalf("post-approval message: HandleMessageWithContext() error = %v", err)
	}
	select {
	case got := <-msgBus.InboundChan():
		// Context.SenderID was pre-set to the raw platform id, so the mirror
		// keeps it; admission (not identity rewriting) is what we assert here.
		if got.SenderID != "stranger" {
			t.Errorf("admitted sender = %q, want stranger", got.SenderID)
		}
		if got.Content != "are you there?" {
			t.Errorf("admitted content = %q", got.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approved sender's message was not admitted onto the bus")
	}
	if len(owner.sent) != noticeCount {
		t.Errorf("approved sender got %d extra pairing notices", len(owner.sent)-noticeCount)
	}
}