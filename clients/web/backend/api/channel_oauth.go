package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kantemba/clawy/pkg/auth"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/logger"
)

const (
	// channelOAuthCallbackPath is where providers redirect the browser after
	// the user approves the install. Register it as the OAuth redirect URL in
	// the Slack / Discord developer console.
	channelOAuthCallbackPath = "/channel/oauth/callback"

	channelOAuthFlowTTL   = 10 * time.Minute
	channelOAuthFlowGCAge = 30 * time.Minute
)

const (
	channelOAuthPending    = "pending"
	channelOAuthSuccess    = "success"
	channelOAuthError      = "error"
	channelOAuthExpired    = "expired"
	channelOAuthNeedsToken = "needs_token"
)

const (
	// slackDefaultOAuthScopes is the minimal bot scope list Clawy's Slack
	// channel actually calls. Anything else (for example channels:history)
	// must already be configured on the Slack app, otherwise Slack rejects the
	// authorize request with invalid_scope.
	slackDefaultOAuthScopes = "chat:write,files:write,reactions:write"

	// discordDefaultPermissions is VIEW_CHANNEL | SEND_MESSAGES |
	// ATTACH_FILES | EMBED_LINKS | READ_MESSAGE_HISTORY |
	// SEND_MESSAGES_IN_THREADS — enough for the Discord channel to talk in
	// servers and threads.
	discordDefaultPermissions = "274878024704"
)

var (
	slackOAuthAuthorizeURL   = "https://slack.com/oauth/v2/authorize"
	slackOAuthTokenURL       = "https://slack.com/api/oauth.v2.access"
	slackOAuthVerifyURL      = "https://slack.com/api/auth.test"
	discordOAuthAuthorizeURL = "https://discord.com/oauth2/authorize"
	discordOAuthTokenURL     = "https://discord.com/api/oauth2/token"
	discordOAuthVerifyURL    = "https://discord.com/api/v10/users/@me"

	channelOAuthHTTPClient = &http.Client{Timeout: 20 * time.Second}

	channelOAuthNow             = time.Now
	channelOAuthGenerateState   = auth.GenerateState
	channelOAuthExchangeSlack   = exchangeSlackChannelCode
	channelOAuthExchangeDiscord = exchangeDiscordChannelCode
)

// channelOAuthFlow tracks one in-flight channel OAuth authorization.
type channelOAuthFlow struct {
	ID           string
	Channel      string
	ClientID     string
	ClientSecret string
	Scopes       string
	Permissions  string
	RedirectURI  string
	OAuthState   string
	Status       string
	Error        string
	AccountID    string
	Token        string // cleared as soon as it has been persisted
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    time.Time
}

// channelOAuthResult is what a provider exchange produced for a flow.
type channelOAuthResult struct {
	// Token is the channel token to persist (Slack bot token, Discord bot token).
	Token string
	// AccountID identifies the bound bot/workspace/guild for display purposes.
	AccountID string
	// NeedsToken reports that the provider completed the authorization but did
	// not issue a token usable by the channel; the user must paste one.
	NeedsToken bool
	Message    string
}

type channelOAuthFlowResponse struct {
	FlowID    string `json:"flow_id"`
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	AccountID string `json:"account_id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Error     string `json:"error,omitempty"`
	// AuthURL / RedirectURI are only returned when a flow is started.
	AuthURL     string `json:"auth_url,omitempty"`
	RedirectURI string `json:"redirect_uri,omitempty"`
}

// registerChannelOAuthRoutes binds the channel OAuth connect endpoints.
func (h *Handler) registerChannelOAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/channels/{name}/oauth/start", h.handleStartChannelOAuth)
	mux.HandleFunc("GET /api/channels/{name}/oauth/flows/{id}", h.handleGetChannelOAuthFlow)
	mux.HandleFunc("GET "+channelOAuthCallbackPath, h.handleChannelOAuthCallback)
}

// handleStartChannelOAuth begins an authorization-code flow for a channel.
//
//	POST /api/channels/{name}/oauth/start
func (h *Handler) handleStartChannelOAuth(w http.ResponseWriter, r *http.Request) {
	channelName := strings.TrimSpace(r.PathValue("name"))
	authorizeURL, supportsOAuth := channelOAuthAuthorizeEndpoint(channelName)
	if !supportsOAuth {
		http.Error(w, "Channel does not support OAuth", http.StatusNotFound)
		return
	}

	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Scopes       string `json:"scopes"`
		Permissions  string `json:"permissions"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load config", http.StatusInternalServerError)
		return
	}
	stored := channelOAuthStoredCredentials(cfg, channelName)

	clientID := firstNonEmptyOAuthValue(req.ClientID, stored.ClientID)
	clientSecret := firstNonEmptyOAuthValue(req.ClientSecret, stored.ClientSecret)
	if clientID == "" {
		http.Error(w, "client_id is required to start an OAuth connect", http.StatusBadRequest)
		return
	}
	if clientSecret == "" {
		http.Error(w, "client_secret is required to start an OAuth connect", http.StatusBadRequest)
		return
	}

	flow := &channelOAuthFlow{
		Channel:      channelName,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  channelOAuthRedirectURI(r),
		Status:       channelOAuthPending,
	}
	switch channelName {
	case config.ChannelSlack:
		flow.Scopes = firstNonEmptyOAuthValue(req.Scopes, stored.Scopes, slackDefaultOAuthScopes)
	case config.ChannelDiscord:
		flow.Permissions = firstNonEmptyOAuthValue(req.Permissions, stored.Permissions, discordDefaultPermissions)
	}

	state, err := channelOAuthGenerateState()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to generate state: %v", err), http.StatusInternalServerError)
		return
	}
	flow.OAuthState = state

	now := channelOAuthNow()
	flow.ID = newChannelOAuthFlowID()
	flow.CreatedAt = now
	flow.UpdatedAt = now
	flow.ExpiresAt = now.Add(channelOAuthFlowTTL)
	h.storeChannelOAuthFlow(flow)

	authURL, err := buildChannelAuthorizeURL(authorizeURL, flow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logger.InfoCF("channel_oauth", "OAuth flow started", map[string]any{
		"channel": channelName,
		"flow_id": flow.ID,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(channelOAuthFlowResponse{
		FlowID:      flow.ID,
		Channel:     flow.Channel,
		Status:      flow.Status,
		ExpiresAt:   flow.ExpiresAt.Format(time.RFC3339),
		AuthURL:     authURL,
		RedirectURI: flow.RedirectURI,
	})
}

// handleGetChannelOAuthFlow reports the status of one connect flow.
//
//	GET /api/channels/{name}/oauth/flows/{id}
func (h *Handler) handleGetChannelOAuthFlow(w http.ResponseWriter, r *http.Request) {
	flowID := strings.TrimSpace(r.PathValue("id"))
	if flowID == "" {
		http.Error(w, "missing flow id", http.StatusBadRequest)
		return
	}

	flow, ok := h.getChannelOAuthFlow(flowID)
	if !ok || flow.Channel != strings.TrimSpace(r.PathValue("name")) {
		http.Error(w, "flow not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(channelOAuthFlowToResponse(flow))
}

// handleChannelOAuthCallback finishes the provider redirect: it exchanges the
// authorization code, stores the resulting token, and sends the browser back
// to the channel page.
//
//	GET /channel/oauth/callback
func (h *Handler) handleChannelOAuthCallback(w http.ResponseWriter, r *http.Request) {
	render := h.renderChannelOAuthCallbackPage

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		render(w, "", channelOAuthError, "Missing state", "missing_state")
		return
	}

	flow, ok := h.getChannelOAuthFlowByState(state)
	if !ok {
		render(w, "", channelOAuthError, "OAuth flow not found", "flow_not_found")
		return
	}

	if flow.Status != channelOAuthPending {
		render(w, flow.ID, flow.Status, "Flow already completed", flow.Error)
		return
	}

	if errMsg := strings.TrimSpace(r.URL.Query().Get("error")); errMsg != "" {
		if desc := strings.TrimSpace(r.URL.Query().Get("error_description")); desc != "" {
			errMsg += ": " + desc
		}
		h.setChannelOAuthFlowError(flow.ID, errMsg)
		render(w, flow.ID, channelOAuthError, "Authorization failed", errMsg)
		return
	}

	if channelOAuthNow().After(flow.ExpiresAt) {
		h.setChannelOAuthFlowStatus(flow.ID, channelOAuthExpired, "flow expired")
		render(w, flow.ID, channelOAuthExpired, "Authorization expired", "flow expired")
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		h.setChannelOAuthFlowError(flow.ID, "missing authorization code")
		render(w, flow.ID, channelOAuthError, "Missing authorization code", "missing_code")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	result, err := h.completeChannelOAuth(ctx, flow, code)
	if err != nil {
		h.setChannelOAuthFlowError(flow.ID, err.Error())
		logger.ErrorCF("channel_oauth", "OAuth exchange failed", map[string]any{
			"channel": flow.Channel,
			"flow_id": flow.ID,
			"error":   err.Error(),
		})
		render(w, flow.ID, channelOAuthError, "Token exchange failed", err.Error())
		return
	}

	if result.NeedsToken {
		h.setChannelOAuthFlowResult(flow.ID, channelOAuthNeedsToken, result.AccountID, "")
		logger.InfoCF("channel_oauth", "OAuth authorized without usable token", map[string]any{
			"channel": flow.Channel,
			"flow_id": flow.ID,
		})
		render(
			w, flow.ID, channelOAuthNeedsToken,
			"Authorized — token still required", result.Message,
		)
		return
	}

	if saveErr := h.saveChannelOAuthToken(flow.Channel, result.Token, result.AccountID); saveErr != nil {
		h.setChannelOAuthFlowError(flow.ID, fmt.Sprintf("failed to save token: %v", saveErr))
		render(w, flow.ID, channelOAuthError, "Failed to save token", saveErr.Error())
		return
	}

	h.setChannelOAuthFlowResult(flow.ID, channelOAuthSuccess, result.AccountID, "")
	logger.InfoCF("channel_oauth", "OAuth connect saved token", map[string]any{
		"channel": flow.Channel,
		"flow_id": flow.ID,
	})
	render(w, flow.ID, channelOAuthSuccess, "Authentication successful", "")
}

// completeChannelOAuth exchanges and verifies the authorization code.
// The provider-specific implementations are indirected through variables so
// tests can stub them.
func (h *Handler) completeChannelOAuth(ctx context.Context, flow *channelOAuthFlow, code string) (channelOAuthResult, error) {
	switch flow.Channel {
	case config.ChannelSlack:
		return channelOAuthExchangeSlack(ctx, flow, code)
	case config.ChannelDiscord:
		return channelOAuthExchangeDiscord(ctx, flow, code)
	default:
		return channelOAuthResult{}, fmt.Errorf("channel %q does not support OAuth", flow.Channel)
	}
}

// exchangeSlackChannelCode trades the code for a bot token and verifies it
// with auth.test before it is persisted.
func exchangeSlackChannelCode(ctx context.Context, flow *channelOAuthFlow, code string) (channelOAuthResult, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {flow.ClientID},
		"client_secret": {flow.ClientSecret},
		"redirect_uri":  {flow.RedirectURI},
	}

	body, err := channelOAuthPostForm(ctx, slackOAuthTokenURL, form)
	if err != nil {
		return channelOAuthResult{}, fmt.Errorf("slack token exchange: %w", err)
	}

	var tokenResp struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
		BotUserID   string `json:"bot_user_id"`
		Team        struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return channelOAuthResult{}, fmt.Errorf("slack token response: %w", err)
	}
	if !tokenResp.OK {
		if tokenResp.Error != "" {
			return channelOAuthResult{}, fmt.Errorf("slack token exchange failed: %s", tokenResp.Error)
		}
		return channelOAuthResult{}, fmt.Errorf("slack token exchange failed: %s", truncateOAuthBody(body))
	}
	if tokenResp.AccessToken == "" {
		return channelOAuthResult{}, fmt.Errorf("slack token exchange returned no access_token")
	}

	accountID := tokenResp.BotUserID
	if accountID == "" {
		accountID = tokenResp.Team.ID
	}

	botID, err := verifySlackChannelToken(ctx, tokenResp.AccessToken)
	if err != nil {
		return channelOAuthResult{}, fmt.Errorf("slack rejected the bot token: %w", err)
	}
	if botID != "" {
		accountID = botID
	}

	return channelOAuthResult{Token: tokenResp.AccessToken, AccountID: accountID}, nil
}

// verifySlackChannelToken calls auth.test and returns the bot ID when the
// token belongs to a bot user.
func verifySlackChannelToken(ctx context.Context, token string) (string, error) {
	body, err := channelOAuthPostForm(ctx, slackOAuthVerifyURL, url.Values{"token": {token}})
	if err != nil {
		return "", err
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		BotID  string `json:"bot_id"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("auth.test response: %w", err)
	}
	if !resp.OK {
		if resp.Error != "" {
			return "", fmt.Errorf("auth.test: %s", resp.Error)
		}
		return "", fmt.Errorf("auth.test: %s", truncateOAuthBody(body))
	}
	return resp.BotID, nil
}

// exchangeDiscordChannelCode trades the code for an access token. Discord only
// issues bot tokens through the developer portal, so the token is verified as
// a bot token first; when Discord returns a bearer token instead, the install
// still succeeded but the user has to paste the bot token.
func exchangeDiscordChannelCode(ctx context.Context, flow *channelOAuthFlow, code string) (channelOAuthResult, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {flow.RedirectURI},
		"client_id":     {flow.ClientID},
		"client_secret": {flow.ClientSecret},
	}

	body, err := channelOAuthPostForm(ctx, discordOAuthTokenURL, form)
	if err != nil {
		return channelOAuthResult{}, fmt.Errorf("discord token exchange: %w", err)
	}

	var tokenResp struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Guild            struct {
			ID string `json:"id"`
		} `json:"guild"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return channelOAuthResult{}, fmt.Errorf("discord token response: %w", err)
	}
	if tokenResp.Error != "" {
		msg := tokenResp.Error
		if tokenResp.ErrorDescription != "" {
			msg += ": " + tokenResp.ErrorDescription
		}
		return channelOAuthResult{}, fmt.Errorf("discord token exchange failed: %s", msg)
	}
	if tokenResp.AccessToken == "" {
		return channelOAuthResult{}, fmt.Errorf("discord token exchange returned no access_token")
	}

	accountID, botUsable, err := verifyDiscordChannelToken(ctx, tokenResp.AccessToken, tokenResp.Guild.ID)
	if err != nil {
		return channelOAuthResult{}, fmt.Errorf("discord rejected the token: %w", err)
	}
	if botUsable {
		return channelOAuthResult{Token: tokenResp.AccessToken, AccountID: accountID}, nil
	}

	return channelOAuthResult{
		NeedsToken: true,
		AccountID:  accountID,
		Message:    "Discord authorized the app but did not return a bot token; paste the bot token from the developer portal to finish.",
	}, nil
}

// verifyDiscordChannelToken reports whether token can authenticate as the bot.
// It returns the bot user ID when usable, otherwise the guild ID (or empty).
func verifyDiscordChannelToken(ctx context.Context, token, guildID string) (string, bool, error) {
	status, body, err := channelOAuthGet(ctx, discordOAuthVerifyURL, "Bot "+token)
	if err != nil {
		return "", false, err
	}
	if status == http.StatusOK {
		var me struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &me); err == nil && me.ID != "" {
			return me.ID, true, nil
		}
		return guildID, true, nil
	}

	// Not a bot token — check whether it is a plain bearer token so the user
	// gets an actionable message instead of a generic failure.
	status, _, err = channelOAuthGet(ctx, discordOAuthVerifyURL, "Bearer "+token)
	if err != nil {
		return "", false, err
	}
	if status == http.StatusOK {
		return guildID, false, nil
	}

	return "", false, fmt.Errorf("unexpected response %d", status)
}

// saveChannelOAuthToken writes the token into the channel settings, enables
// the channel, and restarts the gateway when it is running.
func (h *Handler) saveChannelOAuthToken(channelName, token, accountID string) error {
	if token == "" {
		return fmt.Errorf("empty token")
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	bc := cfg.Channels.Get(channelName)
	if bc == nil {
		bc = &config.Channel{Type: channelName}
		cfg.Channels[channelName] = bc
	}
	bc.Enabled = true

	switch channelName {
	case config.ChannelSlack:
		var slackCfg config.SlackSettings
		if err := bc.Decode(&slackCfg); err != nil {
			return fmt.Errorf("decode slack settings: %w", err)
		}
		slackCfg.BotToken = *config.NewSecureString(token)
	case config.ChannelDiscord:
		var discordCfg config.DiscordSettings
		if err := bc.Decode(&discordCfg); err != nil {
			return fmt.Errorf("decode discord settings: %w", err)
		}
		discordCfg.Token = *config.NewSecureString(token)
	default:
		return fmt.Errorf("channel %q does not support OAuth", channelName)
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		return err
	}

	status := h.gatewayStatusData()
	gatewayStatus, _ := status["gateway_status"].(string)
	if gatewayStatus != "running" {
		return nil
	}

	if _, err := h.RestartGateway(); err != nil {
		logger.ErrorCF("channel_oauth", "failed to restart gateway after saving token", map[string]any{
			"channel": channelName,
			"error":   err.Error(),
		})
	}
	return nil
}

type channelOAuthCredentials struct {
	ClientID     string
	ClientSecret string
	Scopes       string
	Permissions  string
}

// channelOAuthStoredCredentials reads the saved app credentials for a channel.
func channelOAuthStoredCredentials(cfg *config.Config, channelName string) channelOAuthCredentials {
	var creds channelOAuthCredentials
	bc := cfg.Channels.Get(channelName)
	if bc == nil {
		return creds
	}

	switch channelName {
	case config.ChannelSlack:
		var slackCfg config.SlackSettings
		if err := bc.Decode(&slackCfg); err != nil {
			return creds
		}
		creds.ClientID = slackCfg.ClientID
		creds.ClientSecret = slackCfg.ClientSecret.String()
		creds.Scopes = slackCfg.OAuthScopes
	case config.ChannelDiscord:
		var discordCfg config.DiscordSettings
		if err := bc.Decode(&discordCfg); err != nil {
			return creds
		}
		creds.ClientID = discordCfg.ClientID
		creds.ClientSecret = discordCfg.ClientSecret.String()
		creds.Permissions = discordCfg.Permissions
	}
	return creds
}

// buildChannelAuthorizeURL builds the provider authorization URL for a flow.
func buildChannelAuthorizeURL(base string, flow *channelOAuthFlow) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid authorize url: %w", err)
	}

	q := u.Query()
	q.Set("client_id", flow.ClientID)
	q.Set("redirect_uri", flow.RedirectURI)
	q.Set("state", flow.OAuthState)

	switch flow.Channel {
	case config.ChannelSlack:
		q.Set("response_type", "code")
		q.Set("scope", flow.Scopes)
	case config.ChannelDiscord:
		q.Set("response_type", "code")
		q.Set("scope", "bot")
		permissions := flow.Permissions
		if permissions == "" {
			permissions = discordDefaultPermissions
		}
		q.Set("permissions", permissions)
	default:
		return "", fmt.Errorf("channel %q does not support OAuth", flow.Channel)
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// channelOAuthAuthorizeEndpoint returns the provider authorization endpoint
// for a channel, and whether the channel supports OAuth at all.
func channelOAuthAuthorizeEndpoint(channelName string) (string, bool) {
	switch channelName {
	case config.ChannelSlack:
		return slackOAuthAuthorizeURL, true
	case config.ChannelDiscord:
		return discordOAuthAuthorizeURL, true
	default:
		return "", false
	}
}

// channelOAuthRedirectURI is the browser URL providers redirect back to.
func channelOAuthRedirectURI(r *http.Request) string {
	return fmt.Sprintf("%s://%s%s", launcherRequestScheme(r), r.Host, channelOAuthCallbackPath)
}

// channelOAuthPostForm posts an urlencoded body and returns the response body
// for any 2xx status.
func channelOAuthPostForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "clawy")

	resp, err := channelOAuthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned %d: %s", endpoint, resp.StatusCode, truncateOAuthBody(body))
	}
	return body, nil
}

// channelOAuthGet performs a GET with the supplied Authorization header.
func channelOAuthGet(ctx context.Context, endpoint, authorization string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "clawy")

	resp, err := channelOAuthHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func truncateOAuthBody(body []byte) string {
	const max = 300
	s := strings.TrimSpace(string(body))
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func firstNonEmptyOAuthValue(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// renderChannelOAuthCallbackPage reports the result in the provider tab and
// returns the browser to the channel configuration page. The channel is
// resolved from the flow so callers can report early failures (missing or
// unknown state) with the same helper.
func (h *Handler) renderChannelOAuthCallbackPage(w http.ResponseWriter, flowID, status, title, errMsg string) {
	channelName := ""
	if flowID != "" {
		if flow, ok := h.getChannelOAuthFlow(flowID); ok {
			channelName = flow.Channel
		}
	}

	payload := map[string]string{
		"type":   "clawy-channel-oauth-result",
		"flowId": flowID,
		"status": status,
	}
	payloadJSON, _ := json.Marshal(payload)

	message := title
	if errMsg != "" {
		message = fmt.Sprintf("%s: %s", title, errMsg)
	}

	target := "/channels"
	if channelName != "" {
		target += "/" + channelName
	}
	target += "?oauth_flow_id=" + url.QueryEscape(flowID) + "&oauth_status=" + url.QueryEscape(status)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status == channelOAuthSuccess {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}

	_, _ = fmt.Fprintf(
		w,
		"<!doctype html><html><head><meta charset=\"utf-8\"><title>Clawy Channel OAuth</title></head><body><script>(function(){var payload=%s;var hasOpener=false;try{if(window.opener&&!window.opener.closed){window.opener.postMessage(payload,window.location.origin);hasOpener=true}}catch(e){}var target=%s;setTimeout(function(){if(hasOpener){window.close();return}window.location.replace(target)},800)})();</script><div style=\"font-family:Inter,system-ui,sans-serif;padding:24px\"><h2>%s</h2><p>%s</p><p>You can close this window.</p></div></body></html>",
		string(payloadJSON),
		strconv.Quote(target),
		html.EscapeString(title),
		html.EscapeString(message),
	)
}

func channelOAuthFlowToResponse(flow *channelOAuthFlow) channelOAuthFlowResponse {
	resp := channelOAuthFlowResponse{
		FlowID:    flow.ID,
		Channel:   flow.Channel,
		Status:    flow.Status,
		AccountID: flow.AccountID,
		Error:     flow.Error,
	}
	if !flow.ExpiresAt.IsZero() {
		resp.ExpiresAt = flow.ExpiresAt.Format(time.RFC3339)
	}
	return resp
}

func newChannelOAuthFlowID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("cho_%d", time.Now().UnixNano())
	}
	return "cho_" + hex.EncodeToString(buf)
}

func (h *Handler) storeChannelOAuthFlow(flow *channelOAuthFlow) {
	now := channelOAuthNow()
	h.channelOAuthMu.Lock()
	defer h.channelOAuthMu.Unlock()

	h.gcChannelOAuthFlowsLocked(now)
	h.channelOAuthFlows[flow.ID] = flow
	if flow.OAuthState != "" {
		h.channelOAuthState[flow.OAuthState] = flow.ID
	}
}

func (h *Handler) getChannelOAuthFlow(flowID string) (*channelOAuthFlow, bool) {
	now := channelOAuthNow()
	h.channelOAuthMu.Lock()
	defer h.channelOAuthMu.Unlock()

	h.gcChannelOAuthFlowsLocked(now)
	flow, ok := h.channelOAuthFlows[flowID]
	if !ok {
		return nil, false
	}
	cp := *flow
	return &cp, true
}

func (h *Handler) getChannelOAuthFlowByState(state string) (*channelOAuthFlow, bool) {
	now := channelOAuthNow()
	h.channelOAuthMu.Lock()
	defer h.channelOAuthMu.Unlock()

	h.gcChannelOAuthFlowsLocked(now)
	flowID, ok := h.channelOAuthState[state]
	if !ok {
		return nil, false
	}
	flow, ok := h.channelOAuthFlows[flowID]
	if !ok {
		delete(h.channelOAuthState, state)
		return nil, false
	}
	cp := *flow
	return &cp, true
}

func (h *Handler) setChannelOAuthFlowStatus(flowID, status, errMsg string) {
	now := channelOAuthNow()
	h.channelOAuthMu.Lock()
	defer h.channelOAuthMu.Unlock()

	flow, ok := h.channelOAuthFlows[flowID]
	if !ok {
		return
	}
	flow.Status = status
	flow.Error = errMsg
	flow.UpdatedAt = now
	if flow.OAuthState != "" {
		delete(h.channelOAuthState, flow.OAuthState)
	}
}

func (h *Handler) setChannelOAuthFlowError(flowID, errMsg string) {
	h.setChannelOAuthFlowStatus(flowID, channelOAuthError, errMsg)
}

func (h *Handler) setChannelOAuthFlowResult(flowID, status, accountID, errMsg string) {
	now := channelOAuthNow()
	h.channelOAuthMu.Lock()
	defer h.channelOAuthMu.Unlock()

	flow, ok := h.channelOAuthFlows[flowID]
	if !ok {
		return
	}
	flow.Status = status
	flow.Error = errMsg
	flow.AccountID = accountID
	flow.Token = ""
	flow.UpdatedAt = now
	if flow.OAuthState != "" {
		delete(h.channelOAuthState, flow.OAuthState)
	}
}

func (h *Handler) gcChannelOAuthFlowsLocked(now time.Time) {
	for id, flow := range h.channelOAuthFlows {
		if flow.Status == channelOAuthPending && now.After(flow.ExpiresAt) {
			flow.Status = channelOAuthExpired
			flow.Error = "flow expired"
			flow.Token = ""
			flow.UpdatedAt = now
			if flow.OAuthState != "" {
				delete(h.channelOAuthState, flow.OAuthState)
			}
		}
		if flow.Status != channelOAuthPending && now.Sub(flow.UpdatedAt) > channelOAuthFlowGCAge {
			if flow.OAuthState != "" {
				delete(h.channelOAuthState, flow.OAuthState)
			}
			delete(h.channelOAuthFlows, id)
		}
	}
}
