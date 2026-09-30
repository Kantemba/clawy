package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/pkg/config"
)

// startChannelOAuthFlow posts the start endpoint and returns the decoded
// response body plus the state the flow was created with.
func startChannelOAuthFlow(
	t *testing.T,
	mux *http.ServeMux,
	channelName string,
	body string,
) (int, channelOAuthFlowResponse, string) {
	t.Helper()

	state := "test-state-" + channelName
	originalGenerateState := channelOAuthGenerateState
	channelOAuthGenerateState = func() (string, error) { return state, nil }
	t.Cleanup(func() { channelOAuthGenerateState = originalGenerateState })

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/channels/"+channelName+"/oauth/start",
		strings.NewReader(body),
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return rec.Code, channelOAuthFlowResponse{}, state
	}

	var resp channelOAuthFlowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body=%s", err, rec.Body.String())
	}
	return rec.Code, resp, state
}

func TestStartChannelOAuthBuildsSlackAuthorizeURL(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	code, resp, state := startChannelOAuthFlow(t, mux, config.ChannelSlack, `{
		"client_id": "111.222",
		"client_secret": "slack-secret",
		"scopes": "chat:write,channels:read"
	}`)
	if code != http.StatusOK {
		t.Fatalf("start status = %d, want %d", code, http.StatusOK)
	}
	if resp.FlowID == "" {
		t.Fatalf("flow_id = empty, want a value")
	}
	if resp.Status != channelOAuthPending {
		t.Fatalf("status = %q, want %q", resp.Status, channelOAuthPending)
	}
	if resp.RedirectURI == "" || !strings.HasSuffix(resp.RedirectURI, channelOAuthCallbackPath) {
		t.Fatalf("redirect_uri = %q, want suffix %q", resp.RedirectURI, channelOAuthCallbackPath)
	}

	authURL, err := url.Parse(resp.AuthURL)
	if err != nil {
		t.Fatalf("parse auth_url: %v", err)
	}
	if authURL.Host != "slack.com" {
		t.Fatalf("auth_url host = %q, want slack.com", authURL.Host)
	}
	q := authURL.Query()
	if got := q.Get("client_id"); got != "111.222" {
		t.Fatalf("client_id = %q, want %q", got, "111.222")
	}
	if got := q.Get("redirect_uri"); got != resp.RedirectURI {
		t.Fatalf("redirect_uri = %q, want %q", got, resp.RedirectURI)
	}
	if got := q.Get("state"); got != state {
		t.Fatalf("state = %q, want %q", got, state)
	}
	if got := q.Get("scope"); got != "chat:write,channels:read" {
		t.Fatalf("scope = %q, want %q", got, "chat:write,channels:read")
	}
}

func TestStartChannelOAuthUsesDefaultScopesAndStoredCredentials(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := &config.Channel{Type: config.ChannelSlack, Enabled: true}
	bc.Settings = config.RawNode(`{"client_id":"111.222","client_secret":"slack-secret"}`)
	cfg.Channels[config.ChannelSlack] = bc
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Empty body: credentials must come from the saved channel config, and the
	// scope list must fall back to the built-in default.
	code, resp, _ := startChannelOAuthFlow(t, mux, config.ChannelSlack, `{}`)
	if code != http.StatusOK {
		t.Fatalf("start status = %d, want %d", code, http.StatusOK)
	}

	authURL, err := url.Parse(resp.AuthURL)
	if err != nil {
		t.Fatalf("parse auth_url: %v", err)
	}
	q := authURL.Query()
	if got := q.Get("scope"); got != slackDefaultOAuthScopes {
		t.Fatalf("scope = %q, want %q", got, slackDefaultOAuthScopes)
	}
	if got := q.Get("client_id"); got != "111.222" {
		t.Fatalf("client_id = %q, want stored %q", got, "111.222")
	}
}

func TestStartChannelOAuthRequiresCredentials(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/channels/"+config.ChannelSlack+"/oauth/start",
		strings.NewReader(`{"client_id":"111.222"}`),
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "client_secret") {
		t.Fatalf("body = %q, want mention of client_secret", rec.Body.String())
	}
}

func TestStartChannelOAuthRejectsUnsupportedChannel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/channels/weixin/oauth/start",
		strings.NewReader(`{"client_id":"x","client_secret":"y"}`),
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestChannelOAuthCallbackSavesSlackTokenAndEnablesChannel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()
	resetGatewayTestState(t)

	originalExchange := channelOAuthExchangeSlack
	channelOAuthExchangeSlack = func(
		_ context.Context,
		_ *channelOAuthFlow,
		_ string,
	) (channelOAuthResult, error) {
		return channelOAuthResult{
			Token:     "xoxb-saved-bot-token",
			AccountID: "T0123",
		}, nil
	}
	t.Cleanup(func() { channelOAuthExchangeSlack = originalExchange })

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	code, _, state := startChannelOAuthFlow(t, mux, config.ChannelSlack, `{
		"client_id": "111.222",
		"client_secret": "slack-secret"
	}`)
	if code != http.StatusOK {
		t.Fatalf("start status = %d, want %d", code, http.StatusOK)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/channel/oauth/callback?state="+url.QueryEscape(state)+"&code=auth-code",
		nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("callback status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	savedCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := savedCfg.Channels[config.ChannelSlack]
	if bc == nil {
		t.Fatalf("slack channel missing after callback")
	}
	if !bc.Enabled {
		t.Fatalf("slack Enabled = false, want true")
	}
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	slackCfg := decoded.(*config.SlackSettings)
	if got := slackCfg.BotToken.String(); got != "xoxb-saved-bot-token" {
		t.Fatalf("BotToken = %q, want %q", got, "xoxb-saved-bot-token")
	}
}

func TestChannelOAuthCallbackDiscordNeedsTokenKeepsConfig(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()
	resetGatewayTestState(t)

	originalExchange := channelOAuthExchangeDiscord
	channelOAuthExchangeDiscord = func(
		_ context.Context,
		_ *channelOAuthFlow,
		_ string,
	) (channelOAuthResult, error) {
		return channelOAuthResult{
			NeedsToken: true,
			AccountID:  "guild-1",
			Message:    "Discord issued a user token; paste the bot token instead",
		}, nil
	}
	t.Cleanup(func() { channelOAuthExchangeDiscord = originalExchange })

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	code, _, state := startChannelOAuthFlow(t, mux, config.ChannelDiscord, `{
		"client_id": "999",
		"client_secret": "discord-secret"
	}`)
	if code != http.StatusOK {
		t.Fatalf("start status = %d, want %d", code, http.StatusOK)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/channel/oauth/callback?state="+url.QueryEscape(state)+"&code=auth-code",
		nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "paste") {
		t.Fatalf("body = %q, want paste fallback guidance", rec.Body.String())
	}

	savedCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if bc := savedCfg.Channels[config.ChannelDiscord]; bc != nil && bc.Enabled {
		t.Fatalf("discord Enabled = true, want untouched config when no token was issued")
	}
}

func TestChannelOAuthCallbackUnknownState(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/channel/oauth/callback?state=nope", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChannelOAuthCallbackProviderError(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	code, _, state := startChannelOAuthFlow(t, mux, config.ChannelSlack, `{
		"client_id": "111.222",
		"client_secret": "slack-secret"
	}`)
	if code != http.StatusOK {
		t.Fatalf("start status = %d, want %d", code, http.StatusOK)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/channel/oauth/callback?state="+url.QueryEscape(state)+
			"&error=access_denied&error_description=user+denied",
		nil,
	)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	flowID := ""
	if idx := strings.Index(rec.Body.String(), `"flowId":"`); idx >= 0 {
		rest := rec.Body.String()[idx+len(`"flowId":"`):]
		if end := strings.Index(rest, `"`); end >= 0 {
			flowID = rest[:end]
		}
	}
	if flowID == "" {
		t.Fatalf("callback body missing flow id: %s", rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/api/channels/"+config.ChannelSlack+"/oauth/flows/"+flowID,
		nil,
	)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flow status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "access_denied") {
		t.Fatalf("flow body = %q, want access_denied", rec.Body.String())
	}
}

func TestChannelOAuthStoredCredentialsReadsChannelConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Channels[config.ChannelDiscord] = &config.Channel{
		Type:    config.ChannelDiscord,
		Enabled: true,
		Settings: config.RawNode(`{
			"client_id": "discord-app",
			"client_secret": "discord-secret",
			"permissions": "8"
		}`),
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	creds := channelOAuthStoredCredentials(loaded, config.ChannelDiscord)
	if creds.ClientID != "discord-app" {
		t.Fatalf("ClientID = %q, want %q", creds.ClientID, "discord-app")
	}
	if creds.ClientSecret != "discord-secret" {
		t.Fatalf("ClientSecret = %q, want %q", creds.ClientSecret, "discord-secret")
	}
	if creds.Permissions != "8" {
		t.Fatalf("Permissions = %q, want %q", creds.Permissions, "8")
	}
}
