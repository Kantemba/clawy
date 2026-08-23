package pairing

import (
	"errors"
	"testing"
	"time"
)

func TestRequestOrPendingCreatesAndDeduplicates(t *testing.T) {
	m := NewManager(t.TempDir())

	req, created, err := m.RequestOrPending("telegram", "telegram:123", "Alice")
	if err != nil {
		t.Fatalf("RequestOrPending() error = %v", err)
	}
	if !created {
		t.Fatal("first request should be created")
	}
	if len(req.Code) != codeLengthForTest() {
		t.Fatalf("code %q should have %d digits", req.Code, 6)
	}
	if req.Status != StatusPending {
		t.Fatalf("status = %q, want pending", req.Status)
	}

	req2, created, err := m.RequestOrPending("telegram", "telegram:123", "Alice")
	if err != nil {
		t.Fatalf("second RequestOrPending() error = %v", err)
	}
	if created {
		t.Fatal("second request should be deduplicated")
	}
	if req2.Code != req.Code {
		t.Fatalf("deduplicated request code = %q, want %q", req2.Code, req.Code)
	}
}

func codeLengthForTest() int { return 6 }

func TestApproveFlow(t *testing.T) {
	m := NewManager(t.TempDir())

	req, _, err := m.RequestOrPending("discord", "discord:42", "")
	if err != nil {
		t.Fatalf("RequestOrPending() error = %v", err)
	}

	got, err := m.Approve("discord", req.Code)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if got.Status != StatusApproved {
		t.Fatalf("status = %q, want approved", got.Status)
	}

	pending, err := m.Pending()
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d requests, want 0", len(pending))
	}
}

func TestRejectFlow(t *testing.T) {
	m := NewManager(t.TempDir())

	req, _, err := m.RequestOrPending("slack", "slack:7", "")
	if err != nil {
		t.Fatalf("RequestOrPending() error = %v", err)
	}

	got, err := m.Reject("", req.Code)
	if err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if got.Status != StatusRejected {
		t.Fatalf("status = %q, want rejected", got.Status)
	}
}

func TestApproveUnknownCode(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Approve("telegram", "000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Approve(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestChannelScopedApproval(t *testing.T) {
	m := NewManager(t.TempDir())

	req, _, err := m.RequestOrPending("telegram", "telegram:1", "")
	if err != nil {
		t.Fatalf("RequestOrPending() error = %v", err)
	}

	// Wrong channel must not match.
	if _, err := m.Approve("discord", req.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Approve(wrong channel) error = %v, want ErrNotFound", err)
	}
	// Empty channel matches any channel.
	if _, err := m.Approve("", req.Code); err != nil {
		t.Fatalf("Approve(any channel) error = %v", err)
	}
}

func TestExpiredRequestsArePruned(t *testing.T) {
	m := NewManager(t.TempDir())

	req, _, err := m.RequestOrPending("telegram", "telegram:9", "")
	if err != nil {
		t.Fatalf("RequestOrPending() error = %v", err)
	}

	// Force the request past its TTL directly on disk.
	state := &file{Requests: []*Request{{
		Code:      req.Code,
		Channel:   "telegram",
		SenderID:  "telegram:9",
		Status:    StatusPending,
		CreatedAt: time.Now().UTC().Add(-2 * codeTTL),
		UpdatedAt: time.Now().UTC().Add(-2 * codeTTL),
	}}}
	if err := m.save(state); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	if _, err := m.Approve("telegram", req.Code); !errors.Is(err, ErrExpired) {
		t.Fatalf("Approve(expired) error = %v, want ErrExpired", err)
	}
}

func TestPersistenceAcrossManagers(t *testing.T) {
	dir := t.TempDir()
	m1 := NewManager(dir)

	req, _, err := m1.RequestOrPending("vk", "vk:5", "Bob")
	if err != nil {
		t.Fatalf("RequestOrPending() error = %v", err)
	}

	m2 := NewManager(dir)
	got, err := m2.Approve("vk", req.Code)
	if err != nil {
		t.Fatalf("Approve() with fresh manager error = %v", err)
	}
	if got.SenderID != "vk:5" || got.DisplayName != "Bob" {
		t.Fatalf("restored request = %+v", got)
	}
}
