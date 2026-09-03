// Clawy - Ultra-lightweight personal AI agent
// DingTalk voice-note download support.

package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/h2non/filetype"

	"github.com/Kantemba/clawy/pkg/logger"
	"github.com/Kantemba/clawy/pkg/media"
)

const (
	dingTalkAPIBase       = "https://api.dingtalk.com"
	tokenRefreshMargin    = 60 * time.Second
	voiceHTTPTimeout      = 60 * time.Second
	voiceMaxBytes         = 25 << 20 // 25 MiB safety cap
	voiceFallbackExt      = ".amr"
	voiceFallbackCT       = "audio/amr"
)

// accessToken returns a cached enterprise access token, requesting a fresh
// one from the OAuth endpoint when the cached value is missing or stale.
func (c *DingTalkChannel) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.apiToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.apiToken, nil
	}

	body, err := json.Marshal(map[string]string{
		"appKey":    c.clientID,
		"appSecret": c.clientSecret,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dingTalkAPIBase+"/v1.0/oauth2/accessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.mediaHTTP().Do(req)
	if err != nil {
		return "", fmt.Errorf("request access token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("access token returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode access token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("empty access token")
	}

	c.apiToken = parsed.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(parsed.ExpireIn) * time.Second - tokenRefreshMargin)
	return c.apiToken, nil
}

// resolveMediaDownloadURL exchanges a bot-message downloadCode for a
// temporary download URL.
func (c *DingTalkChannel) resolveMediaDownloadURL(ctx context.Context, downloadCode string) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]string{
		"robotCode":    c.clientID,
		"downloadCode": downloadCode,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dingTalkAPIBase+"/v1.0/robot/messageFiles/download", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := c.mediaHTTP().Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve media download url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("media download url returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode media download response: %w", err)
	}
	if parsed.DownloadURL == "" {
		return "", fmt.Errorf("empty media download url")
	}
	return parsed.DownloadURL, nil
}

// downloadVoiceMessage resolves and downloads a DingTalk voice note, stores
// it with the media store, appends the ref to mediaRefs, and returns true on
// success.
func (c *DingTalkChannel) downloadVoiceMessage(
	ctx context.Context,
	downloadCode, scope string,
	mediaRefs *[]string,
) bool {
	store := c.GetMediaStore()
	if store == nil {
		logger.WarnCF("dingtalk", "Voice message received but media store is unavailable", nil)
		return false
	}

	dlCtx, cancel := context.WithTimeout(ctx, voiceHTTPTimeout)
	defer cancel()

	url, err := c.resolveMediaDownloadURL(dlCtx, downloadCode)
	if err != nil {
		logger.WarnCF("dingtalk", "Failed to resolve voice download url", map[string]any{
			"error": err.Error(),
		})
		return false
	}

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		logger.WarnCF("dingtalk", "Invalid voice download request", map[string]any{"error": err.Error()})
		return false
	}
	resp, err := c.mediaHTTP().Do(req)
	if err != nil {
		logger.WarnCF("dingtalk", "Failed to download voice message", map[string]any{"error": err.Error()})
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.WarnCF("dingtalk", "Voice download returned non-200 status", map[string]any{"status": resp.StatusCode})
		return false
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, voiceMaxBytes+1))
	if err != nil {
		logger.WarnCF("dingtalk", "Failed to read voice download", map[string]any{"error": err.Error()})
		return false
	}
	if len(data) > voiceMaxBytes {
		logger.WarnCF("dingtalk", "Voice message too large", nil)
		return false
	}

	filename, contentType := dingtalkVoiceMetadata(data, resp.Header.Get("Content-Type"))
	if err := os.MkdirAll(media.TempDir(), 0o700); err != nil {
		logger.WarnCF("dingtalk", "Failed to create media directory", map[string]any{"error": err.Error()})
		return false
	}
	tmpFile, err := os.CreateTemp(media.TempDir(), "voice-*"+filepath.Ext(filename))
	if err != nil {
		logger.WarnCF("dingtalk", "Failed to create temp file", map[string]any{"error": err.Error()})
		return false
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		logger.WarnCF("dingtalk", "Failed to write voice message", map[string]any{"error": err.Error()})
		return false
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		logger.WarnCF("dingtalk", "Failed to close voice message file", map[string]any{"error": err.Error()})
		return false
	}

	ref, err := store.Store(tmpPath, media.MediaMeta{
		Filename:      filename,
		ContentType:   contentType,
		Source:        "dingtalk",
		CleanupPolicy: media.CleanupPolicyDeleteOnCleanup,
	}, scope)
	if err != nil {
		_ = os.Remove(tmpPath)
		logger.WarnCF("dingtalk", "Failed to register voice message", map[string]any{"error": err.Error()})
		return false
	}

	*mediaRefs = append(*mediaRefs, ref)
	return true
}

// dingtalkVoiceMetadata sniffs the downloaded voice payload so the correct
// audio extension/content type reaches the transcription pipeline. DingTalk
// does not document the recording format, so detection beats guessing.
func dingtalkVoiceMetadata(data []byte, headerCT string) (string, string) {
	filename := "voice" + voiceFallbackExt
	contentType := voiceFallbackCT

	if kind, err := filetype.Match(data); err == nil && kind != filetype.Unknown {
		ext := "." + kind.Extension
		if exts, extErr := mime.ExtensionsByType(kind.MIME.Value); extErr == nil && len(exts) > 0 {
			ext = exts[0]
		}
		filename = "voice" + ext
		contentType = kind.MIME.Value
		return filename, contentType
	}

	if ct := strings.TrimSpace(strings.SplitN(headerCT, ";", 2)[0]); ct != "" {
		if exts, err := mime.ExtensionsByType(ct); err == nil && len(exts) > 0 {
			filename = "voice" + exts[0]
			contentType = ct
		}
	}
	return filename, contentType
}
