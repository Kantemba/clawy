package pairing

// Pairing implements an OpenClaw-style DM pairing flow: when an unknown sender
// sends a direct message to a channel that has an allow-list, Clawy creates a
// pending pairing request with a short numeric code and replies with setup
// instructions. The owner then runs `clawy pairing approve <channel> <code>`
// to add the sender to the channel's allow_from list.

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kantemba/clawy/pkg/fileutil"
	"github.com/Kantemba/clawy/pkg/logger"
)

// Status is the lifecycle state of a pairing request.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
)

const (
	// codeTTL is how long a pending pairing request stays valid.
	codeTTL = time.Hour
	// maxPendingPerChannel caps the number of concurrent pending requests
	// per channel; oldest pending requests are expired beyond this.
	maxPendingPerChannel = 3
)

var (
	// ErrNotFound is returned when no matching request exists for a code.
	ErrNotFound = errors.New("no pairing request found for that code")
	// ErrExpired is returned when a request exists but its TTL elapsed.
	ErrExpired = errors.New("pairing request expired, ask the sender to message again")
)

// Request is a single pairing request record.
type Request struct {
	Code        string    `json:"code"`
	Channel     string    `json:"channel"`
	SenderID    string    `json:"sender_id"` // canonical "platform:id" where available
	DisplayName string    `json:"display_name,omitempty"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// file is the on-disk representation persisted in the workspace.
type file struct {
	Requests []*Request `json:"requests"`
}

// Manager stores and evaluates pairing requests for a workspace.
type Manager struct {
	mu        sync.Mutex
	path      string
	workspace string
}

func generateCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func (m *Manager) load() (*file, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &file{}, nil
		}
		return nil, err
	}
	out := &file{}
	if err := json.Unmarshal(data, out); err != nil {
		logger.WarnCF("pairing", "failed to parse pairing state, starting fresh", map[string]any{
			"path":  m.path,
			"error": err.Error(),
		})
		return &file{}, nil
	}
	return out, nil
}

func (m *Manager) save(state *file) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(m.path, data, 0o600)
}

// NewManager creates a pairing manager rooted at the given workspace.
func NewManager(workspace string) *Manager {
	return &Manager{
		path:      filepath.Join(workspace, "pairing", "pairing.json"),
		workspace: workspace,
	}
}

// Workspace returns the workspace root this manager persists under.
func (m *Manager) Workspace() string { return m.workspace }

// RequestOrPending returns the existing pending request for the given sender or
// creates a new one. created reports whether a fresh request was issued.
func (m *Manager) RequestOrPending(channel, senderID, displayName string) (*Request, bool, error) {
	channel = strings.TrimSpace(channel)
	senderID = strings.TrimSpace(senderID)
	if channel == "" || senderID == "" {
		return nil, false, fmt.Errorf("channel and sender id are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.load()
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	prune(state, now)

	for _, r := range state.Requests {
		if r.Channel == channel && r.SenderID == senderID && r.Status == StatusPending {
			return r, false, nil
		}
	}

	req := &Request{
		Code:        generateCode(),
		Channel:     channel,
		SenderID:    senderID,
		DisplayName: strings.TrimSpace(displayName),
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	state.Requests = append(state.Requests, req)
	if err := m.save(state); err != nil {
		return nil, false, err
	}
	logger.InfoCF("pairing", "created pairing request", map[string]any{
		"channel": channel, "code": req.Code,
	})
	return req, true, nil
}

// Approve marks the request with the given code as approved. When channel is
// non-empty the lookup is restricted to requests from that channel.
func (m *Manager) Approve(channel, code string) (*Request, error) {
	return m.resolve(channel, code, StatusApproved)
}

// Reject marks the request with the given code as rejected.
func (m *Manager) Reject(channel, code string) (*Request, error) {
	return m.resolve(channel, code, StatusRejected)
}

func (m *Manager) resolve(channel, code string, status Status) (*Request, error) {
	code = strings.TrimSpace(code)
	channel = strings.TrimSpace(channel)
	if code == "" {
		return nil, ErrNotFound
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.load()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	prune(state, now)

	for _, r := range state.Requests {
		if r.Code != code {
			continue
		}
		if channel != "" && r.Channel != channel {
			continue
		}
		switch r.Status {
		case StatusApproved:
			return r, nil // idempotent re-approval
		case StatusPending:
			r.Status = status
			r.UpdatedAt = now
			if err := m.save(state); err != nil {
				return nil, err
			}
			logger.InfoCF("pairing", string(status)+" pairing request", map[string]any{
				"channel": r.Channel, "sender_id": r.SenderID,
			})
			return r, nil
		default:
			return nil, ErrExpired
		}
	}
	return nil, ErrNotFound
}

// Pending returns all pending requests across channels, pruning stale ones.
func (m *Manager) Pending() ([]*Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.load()
	if err != nil {
		return nil, err
	}
	prune(state, time.Now().UTC())
	_ = m.save(state)

	var out []*Request
	for _, r := range state.Requests {
		if r.Status == StatusPending {
			out = append(out, r)
		}
	}
	sortRequests(out)
	return out, nil
}

// List returns every known request (any status), newest first.
func (m *Manager) List() ([]*Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.load()
	if err != nil {
		return nil, err
	}
	out := append([]*Request(nil), state.Requests...)
	sortRequests(out)
	return out, nil
}

// IsApproved reports whether an approved pairing request exists for the given
// channel and sender, which admits the sender like an allow-listed user.
func (m *Manager) IsApproved(channel, senderID string) bool {
	channel = strings.TrimSpace(channel)
	senderID = strings.TrimSpace(senderID)
	if channel == "" || senderID == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.load()
	if err != nil {
		return false
	}
	for _, r := range state.Requests {
		if r.Channel == channel && r.SenderID == senderID && r.Status == StatusApproved {
			return true
		}
	}
	return false
}

func sortRequests(reqs []*Request) {
	sort.Slice(reqs, func(i, j int) bool {
		return reqs[i].CreatedAt.After(reqs[j].CreatedAt)
	})
}

// prune expires stale requests and caps pending count per channel.
// Callers must hold the manager lock.
func prune(state *file, now time.Time) {
	pendingPerChannel := map[string]int{}
	for _, r := range state.Requests {
		if r.Status == StatusPending && now.Sub(r.CreatedAt) > codeTTL {
			r.Status = StatusExpired
			r.UpdatedAt = now
		}
	}
	for _, r := range state.Requests {
		if r.Status == StatusPending {
			pendingPerChannel[r.Channel]++
		}
	}
	for _, r := range state.Requests {
		if r.Status != StatusPending {
			continue
		}
		if pendingPerChannel[r.Channel] > maxPendingPerChannel {
			r.Status = StatusExpired
			r.UpdatedAt = now
			pendingPerChannel[r.Channel]--
		}
	}
}
