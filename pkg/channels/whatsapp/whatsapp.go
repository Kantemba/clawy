package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/identity"
	"github.com/Kantemba/clawy/pkg/logger"
	"github.com/Kantemba/clawy/pkg/media"
	"github.com/Kantemba/clawy/pkg/utils"
)

type WhatsAppChannel struct {
	*channels.BaseChannel
	conn      *websocket.Conn
	config    *config.WhatsAppSettings
	url       string
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	connected bool
}

// bridgeAudioAnnotationRe matches existing voice/audio annotations in bridge
// message content so we do not append duplicate tags.
var bridgeAudioAnnotationRe = regexp.MustCompile(`\[(voice|audio)(?::[^\]]*)?\]`)

func NewWhatsAppChannel(
	bc *config.Channel,
	cfg *config.WhatsAppSettings,
	bus *bus.MessageBus,
) (*WhatsAppChannel, error) {
	base := channels.NewBaseChannel(
		"whatsapp",
		cfg,
		bus,
		bc.AllowFrom,
		channels.WithMaxMessageLength(65536),
		channels.WithReasoningChannelID(bc.ReasoningChannelID),
	)

	return &WhatsAppChannel{
		BaseChannel: base,
		config:      cfg,
		url:         cfg.BridgeURL,
		connected:   false,
	}, nil
}

func (c *WhatsAppChannel) Start(ctx context.Context) error {
	logger.InfoCF("whatsapp", "Starting WhatsApp channel", map[string]any{
		"bridge_url": c.url,
	})

	c.ctx, c.cancel = context.WithCancel(ctx)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, resp, err := dialer.Dial(c.url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		c.cancel()
		return fmt.Errorf("failed to connect to WhatsApp bridge: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	c.SetRunning(true)
	logger.InfoC("whatsapp", "WhatsApp channel connected")

	go c.listen()

	return nil
}

func (c *WhatsAppChannel) Stop(ctx context.Context) error {
	logger.InfoC("whatsapp", "Stopping WhatsApp channel...")

	// Cancel context first to signal listen goroutine to exit
	if c.cancel != nil {
		c.cancel()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			logger.ErrorCF("whatsapp", "Error closing WhatsApp connection", map[string]any{
				"error": err.Error(),
			})
		}
		c.conn = nil
	}

	c.connected = false
	c.SetRunning(false)

	return nil
}

func (c *WhatsAppChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}

	// Check ctx before acquiring lock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("whatsapp connection not established: %w", channels.ErrTemporary)
	}

	payload := map[string]any{
		"type":    "message",
		"to":      msg.ChatID,
		"content": msg.Content,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		_ = c.conn.SetWriteDeadline(time.Time{})
		return nil, fmt.Errorf("whatsapp send: %w", channels.ErrTemporary)
	}
	_ = c.conn.SetWriteDeadline(time.Time{})

	return nil, nil
}

func (c *WhatsAppChannel) listen() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()

			if conn == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				logger.ErrorCF("whatsapp", "WhatsApp read error", map[string]any{
					"error": err.Error(),
				})
				time.Sleep(2 * time.Second)
				continue
			}

			var msg map[string]any
			if err := json.Unmarshal(message, &msg); err != nil {
				logger.ErrorCF("whatsapp", "Failed to unmarshal WhatsApp message", map[string]any{
					"error": err.Error(),
				})
				continue
			}

			msgType, ok := msg["type"].(string)
			if !ok {
				continue
			}

			if msgType == "message" {
				c.handleIncomingMessage(msg)
			}
		}
	}
}

func (c *WhatsAppChannel) handleIncomingMessage(msg map[string]any) {
	senderID, ok := msg["from"].(string)
	if !ok {
		return
	}

	chatID, ok := msg["chat"].(string)
	if !ok {
		chatID = senderID
	}

	content, ok := msg["content"].(string)
	if !ok {
		content = ""
	}

	var mediaPaths []string
	if mediaData, ok := msg["media"].([]any); ok {
		mediaPaths = make([]string, 0, len(mediaData))
		for _, m := range mediaData {
			if path, ok := m.(string); ok {
				mediaPaths = append(mediaPaths, path)
			}
		}
	}

	metadata := make(map[string]string)
	var messageID string
	if mid, ok := msg["id"].(string); ok {
		messageID = mid
	}
	if userName, ok := msg["from_name"].(string); ok {
		metadata["user_name"] = userName
	}

	logger.InfoCF("whatsapp", "WhatsApp message received", map[string]any{
		"sender":  senderID,
		"preview": utils.Truncate(content, 50),
	})

	sender := bus.SenderInfo{
		Platform:    "whatsapp",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("whatsapp", senderID),
	}
	if display, ok := metadata["user_name"]; ok {
		sender.DisplayName = display
	}

	if !c.IsAllowedSender(sender) {
		return
	}

	// Register audio files delivered by the bridge with the media store so
	// the transcription pipeline can resolve them; other media keeps the
	// legacy raw-path passthrough.
	audioCount := 0
	scope := channels.BuildMediaScope("whatsapp", chatID, messageID)
	registered := make([]string, 0, len(mediaPaths))
	for _, path := range mediaPaths {
		if utils.IsAudioFile(path, "") {
			if ref := c.registerBridgeAudio(path, scope); ref != "" {
				registered = append(registered, ref)
				audioCount++
				continue
			}
		}
		registered = append(registered, path)
	}
	if audioCount > 0 {
		tag := "[voice]"
		if content == "" {
			content = tag
		} else if !bridgeAudioAnnotationRe.MatchString(content) {
			content += "\n" + tag
		}
	}
	mediaPaths = registered

	inboundCtx := bus.InboundContext{
		Channel:   "whatsapp",
		ChatID:    chatID,
		SenderID:  senderID,
		MessageID: messageID,
		Raw:       metadata,
	}
	if chatID == senderID {
		inboundCtx.ChatType = "direct"
	} else {
		inboundCtx.ChatType = "group"
	}

	c.HandleInboundContext(c.ctx, chatID, content, mediaPaths, inboundCtx, sender)
}

// registerBridgeAudio registers a bridge-provided local audio file with the
// media store and returns the ref. Returns "" when registration is not
// possible. The underlying file is owned by the bridge, so forget-only
// cleanup is used.
func (c *WhatsAppChannel) registerBridgeAudio(path, scope string) string {
	store := c.GetMediaStore()
	if store == nil {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	ref, err := store.Store(path, media.MediaMeta{
		Filename:      filepath.Base(path),
		Source:        "whatsapp",
		CleanupPolicy: media.CleanupPolicyForgetOnly,
	}, scope)
	if err != nil {
		logger.WarnCF("whatsapp", "Failed to register bridge audio with media store", map[string]any{
			"error": err.Error(),
		})
		return ""
	}
	return ref
}
