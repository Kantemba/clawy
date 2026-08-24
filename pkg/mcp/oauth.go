package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
	"golang.org/x/term"

	clawyauth "github.com/Kantemba/clawy/pkg/auth"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/logger"
)

// CredentialProviderPrefix is prepended to MCP server names when storing
// OAuth credentials in the shared encrypted auth store, keeping them separate
// from LLM provider credentials.
const CredentialProviderPrefix = "mcp:"

// CredentialProviderName returns the auth-store provider key for an MCP server.
func CredentialProviderName(serverName string) string {
	return CredentialProviderPrefix + strings.ToLower(strings.TrimSpace(serverName))
}

// oauthLoginTimeout is how long the interactive authorization flow waits for
// the user (browser callback or pasted URL) before giving up.
var oauthLoginTimeout = 5 * time.Minute

// credentialStore abstracts credential persistence so tests can inject an
// in-memory implementation.
type credentialStore interface {
	Get(provider string) (*clawyauth.AuthCredential, error)
	Set(provider string, cred *clawyauth.AuthCredential) error
	Delete(provider string) error
}

type encryptedCredentialStore struct{}

func (encryptedCredentialStore) Get(provider string) (*clawyauth.AuthCredential, error) {
	return clawyauth.GetCredential(provider)
}

func (encryptedCredentialStore) Set(provider string, cred *clawyauth.AuthCredential) error {
	return clawyauth.SetCredential(provider, cred)
}

func (encryptedCredentialStore) Delete(provider string) error {
	return clawyauth.DeleteCredential(provider)
}

// Injected dependencies for testability.
var (
	oauthOpenBrowser = clawyauth.OpenBrowser
	oauthStdin       io.Reader       = os.Stdin
	oauthStdout      io.Writer       = os.Stdout
	oauthCredStore   credentialStore = encryptedCredentialStore{}
	oauthStdinIsTTY                  = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	oauthHTTPClient                  = &http.Client{Timeout: 30 * time.Second}
)

// oauthDiscovery holds everything learned about a protected resource and its
// authorization server.
type oauthDiscovery struct {
	// resource is the RFC 8707 resource identifier sent with authorize and
	// token requests.
	resource string
	// metadata is the authorization server metadata (RFC 8414).
	metadata *oauthex.AuthServerMeta
}

// MCPOAuthHandler implements [auth.OAuthHandler] for a single remote MCP
// server following the MCP authorization specification: protected resource
// metadata discovery (RFC 9728), authorization server metadata discovery
// (RFC 8414), pre-registered or dynamically registered clients (RFC 7591),
// the authorization code flow with PKCE, token persistence and silent
// refresh.
//
// When the redirect URI is on localhost, Clawy starts a local HTTP callback
// listener AND asks the user to paste the full callback URL into the
// terminal; whichever arrives first is used. The OAuth state parameter is
// validated in both paths.
type MCPOAuthHandler struct {
	serverName string
	serverURL  string
	cfg        *config.MCPOAuthConfig

	mu          sync.Mutex
	tokenSource oauth2.TokenSource
	disc        *oauthDiscovery
}

var _ auth.OAuthHandler = (*MCPOAuthHandler)(nil)

// NewOAuthHandler creates an OAuth handler for a remote MCP server. It
// returns (nil, nil) when OAuth is not enabled for the server.
func NewOAuthHandler(serverName string, cfg config.MCPServerConfig) (*MCPOAuthHandler, error) {
	if !cfg.OAuth.IsEnabled() {
		return nil, nil
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("oauth requires a url (sse/http transport) for MCP server %q", serverName)
	}
	return &MCPOAuthHandler{
		serverName: serverName,
		serverURL:  cfg.URL,
		cfg:        cfg.OAuth,
	}, nil
}

// ── auth.OAuthHandler implementation ─────────────────────────────────────────

// TokenSource returns the current token source. It may return (nil, nil) when
// no usable token is available yet; the transport then sends the request
// unauthenticated, and on a 401/403 calls Authorize to start the flow.
func (h *MCPOAuthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if h == nil {
		return nil, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tokenSourceLocked(ctx)
}

func (h *MCPOAuthHandler) tokenSourceLocked(ctx context.Context) (oauth2.TokenSource, error) {
	if h.tokenSource != nil {
		return h.tokenSource, nil
	}
	cred, err := oauthCredStore.Get(CredentialProviderName(h.serverName))
	if err != nil {
		logger.WarnCF("mcp", "Failed to load stored MCP OAuth credential",
			map[string]any{"server": h.serverName, "error": err.Error()})
		return nil, nil
	}
	if cred == nil || cred.AccessToken == "" {
		return nil, nil
	}
	if cred.NeedsRefresh() && cred.RefreshToken != "" {
		refreshed, refreshErr := h.refreshLocked(ctx, cred)
		if refreshErr != nil {
			logger.InfoCF("mcp", "Silent OAuth token refresh failed; re-authorization required",
				map[string]any{"server": h.serverName, "error": refreshErr.Error()})
			return nil, nil
		}
		cred = refreshed
	}
	h.tokenSource = staticTokenSource(cred)
	return h.tokenSource, nil
}

// Authorize implements [auth.OAuthHandler.Authorize]. It is called by the
// transport after receiving a 401/403 response; on success the transport
// retries the request with the freshly obtained token.
func (h *MCPOAuthHandler) Authorize(ctx context.Context, _ *http.Request, resp *http.Response) error {
	if h == nil {
		resp.Body.Close()
		return fmt.Errorf("oauth handler not configured")
	}
	defer resp.Body.Close()

	h.mu.Lock()
	defer h.mu.Unlock()
	return h.authorizeLocked(ctx, resp.Header.Values("WWW-Authenticate"))
}

// AuthorizeInteractive runs the full authorization flow without a triggering
// HTTP response. It backs `clawy mcp auth`.
func (h *MCPOAuthHandler) AuthorizeInteractive(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.authorizeLocked(ctx, nil)
}

// authorizeLocked performs discovery, client resolution, the interactive
// authorization code flow with PKCE, token exchange and persistence. The
// handler mutex must be held.
func (h *MCPOAuthHandler) authorizeLocked(ctx context.Context, wwwAuthenticate []string) error {
	// Try a silent refresh before bothering the user.
	if cred, credErr := oauthCredStore.Get(CredentialProviderName(h.serverName)); credErr == nil &&
		cred != nil && cred.RefreshToken != "" {
		refreshed, refreshErr := h.refreshLocked(ctx, cred)
		if refreshErr == nil {
			h.tokenSource = staticTokenSource(refreshed)
			logger.InfoCF("mcp", "OAuth token refreshed silently",
				map[string]any{"server": h.serverName})
			return nil
		}
		logger.InfoCF("mcp", "Silent OAuth refresh failed; starting interactive authorization",
			map[string]any{"server": h.serverName, "error": refreshErr.Error()})
	}

	disc, err := h.discoverLocked(ctx, wwwAuthenticate)
	if err != nil {
		return fmt.Errorf("OAuth discovery failed for MCP server %q: %w", h.serverName, err)
	}

	redirectURI := h.redirectURI()
	clientID, clientSecret, err := h.resolveClientLocked(ctx, disc, redirectURI)
	if err != nil {
		return fmt.Errorf("OAuth client setup failed for MCP server %q: %w", h.serverName, err)
	}

	conf := h.oauth2Config(disc, clientID, clientSecret, redirectURI)
	verifier := oauth2.GenerateVerifier()
	state, err := generateOAuthState()
	if err != nil {
		return fmt.Errorf("generating OAuth state: %w", err)
	}
	authURL := conf.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("resource", disc.resource),
	)

	fmt.Fprintf(oauthStdout,
		"\nMCP server %q requires authentication.\n\nOpen this URL in your browser to authorize:\n\n  %s\n\n",
		h.serverName, authURL)

	result, flowErr := waitForAuthorizationCode(ctx, waitForAuthorizationCodeOptions{
		authURL:       authURL,
		expectedState: state,
		redirectURI:   redirectURI,
		openBrowser:   !h.cfg.NoBrowser,
		timeout:       oauthLoginTimeout,
		stdin:         oauthStdin,
		stdout:        oauthStdout,
	})
	if flowErr != nil {
		return fmt.Errorf("OAuth authorization failed for MCP server %q: %w", h.serverName, flowErr)
	}

	tok, exchangeErr := conf.Exchange(
		ctx,
		result.Code,
		oauth2.VerifierOption(verifier),
		oauth2.SetAuthURLParam("resource", disc.resource),
	)
	if exchangeErr != nil {
		return fmt.Errorf("OAuth token exchange failed for MCP server %q: %w", h.serverName, exchangeErr)
	}
	if tok.AccessToken == "" {
		return fmt.Errorf("OAuth token exchange returned no access token for MCP server %q", h.serverName)
	}

	cred := &clawyauth.AuthCredential{
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		ExpiresAt:     tok.Expiry,
		Provider:      CredentialProviderName(h.serverName),
		AuthMethod:    "oauth",
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		TokenEndpoint: disc.metadata.TokenEndpoint,
		Scopes:        append([]string(nil), h.cfg.Scopes...),
	}
	if storeErr := oauthCredStore.Set(CredentialProviderName(h.serverName), cred); storeErr != nil {
		return fmt.Errorf("persisting OAuth credential for MCP server %q: %w", h.serverName, storeErr)
	}

	h.tokenSource = staticTokenSource(cred)
	fmt.Fprintf(oauthStdout, "\n✓ Authenticated MCP server %q.\n", h.serverName)
	return nil
}

// ── Discovery / client resolution / refresh ─────────────────────────────────

// discoverLocked performs protected resource and authorization server
// metadata discovery, caching the result on the handler.
func (h *MCPOAuthHandler) discoverLocked(ctx context.Context, wwwAuthenticate []string) (*oauthDiscovery, error) {
	if h.disc != nil {
		return h.disc, nil
	}

	resource := ""
	var issuer string

	if h.cfg.Issuer != "" {
		issuer = strings.TrimRight(strings.TrimSpace(h.cfg.Issuer), "/")
		resource = h.serverURL
	} else {
		prm, prmIssuer, prmErr := fetchProtectedResourceMetadata(ctx, wwwAuthenticate, h.serverURL)
		if prmErr != nil {
			return nil, prmErr
		}
		if prm != nil {
			resource = prm.Resource
			issuer = prmIssuer
		}
	}

	if issuer == "" {
		// Fall back to treating the MCP server origin as the authorization
		// server (common for single-vendor deployments).
		u, err := url.Parse(h.serverURL)
		if err != nil {
			return nil, fmt.Errorf("parsing server URL: %w", err)
		}
		u.Path, u.RawQuery, u.Fragment = "", "", ""
		issuer = strings.TrimRight(u.String(), "/")
	}
	if resource == "" {
		resource = h.serverURL
	}

	meta, metaErr := fetchAuthServerMetadata(ctx, issuer)
	if metaErr != nil {
		return nil, metaErr
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("authorization server %q metadata lacks required endpoints", issuer)
	}

	h.disc = &oauthDiscovery{resource: resource, metadata: meta}
	return h.disc, nil
}

// resolveClientLocked determines which OAuth client identity to use:
// configured credentials, credentials persisted by an earlier dynamic
// registration, or a fresh dynamic client registration (RFC 7591).
func (h *MCPOAuthHandler) resolveClientLocked(
	ctx context.Context,
	disc *oauthDiscovery,
	redirectURI string,
) (string, string, error) {
	if h.cfg.ClientID != "" {
		return h.cfg.ClientID, h.cfg.ClientSecret, nil
	}

	if cred, err := oauthCredStore.Get(CredentialProviderName(h.serverName)); err == nil &&
		cred != nil && cred.ClientID != "" {
		return cred.ClientID, cred.ClientSecret, nil
	}

	endpoint := disc.metadata.RegistrationEndpoint
	if endpoint == "" {
		return "", "", fmt.Errorf(
			"no client_id configured and the authorization server does not support dynamic client "+
				"registration; register a client manually and set tools.mcp.servers.%s.oauth.client_id",
			h.serverName)
	}

	regResp, regErr := oauthex.RegisterClient(ctx, endpoint, &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{redirectURI},
		ClientName:              "Clawy",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   strings.Join(h.cfg.Scopes, " "),
	}, oauthHTTPClient)
	if regErr != nil {
		return "", "", fmt.Errorf("dynamic client registration failed: %w", regErr)
	}
	logger.InfoCF("mcp", "Dynamically registered OAuth client",
		map[string]any{"server": h.serverName, "client_id": regResp.ClientID})
	return regResp.ClientID, regResp.ClientSecret, nil
}

func (h *MCPOAuthHandler) oauth2Config(
	disc *oauthDiscovery,
	clientID, clientSecret, redirectURI string,
) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  disc.metadata.AuthorizationEndpoint,
			TokenURL: disc.metadata.TokenEndpoint,
		},
		RedirectURL: redirectURI,
		Scopes:      h.cfg.Scopes,
	}
}

// redirectURI builds the localhost callback URI advertised during
// authorization.
func (h *MCPOAuthHandler) redirectURI() string {
	return fmt.Sprintf("http://localhost:%d%s",
		h.cfg.EffectiveCallbackPort(),
		h.cfg.EffectiveRedirectPath())
}

// refreshLocked exchanges a stored refresh token for a new access token and
// persists the result.
func (h *MCPOAuthHandler) refreshLocked(
	ctx context.Context,
	cred *clawyauth.AuthCredential,
) (*clawyauth.AuthCredential, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	tokenURL := cred.TokenEndpoint
	if tokenURL == "" {
		disc, err := h.discoverLocked(ctx, nil)
		if err != nil {
			return nil, err
		}
		tokenURL = disc.metadata.TokenEndpoint
	}
	clientID := cred.ClientID
	if clientID == "" {
		clientID = h.cfg.ClientID
	}
	clientSecret := cred.ClientSecret
	if clientSecret == "" {
		clientSecret = h.cfg.ClientSecret
	}
	if clientID == "" {
		return nil, fmt.Errorf("no OAuth client id available for token refresh")
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: tokenURL},
	}
	tok, err := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: cred.RefreshToken}).Token()
	if err != nil {
		return nil, err
	}

	refreshed := *cred
	refreshed.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		refreshed.RefreshToken = tok.RefreshToken
	}
	refreshed.ExpiresAt = tok.Expiry
	if err := oauthCredStore.Set(refreshed.Provider, &refreshed); err != nil {
		return nil, fmt.Errorf("persisting refreshed credential: %w", err)
	}
	return &refreshed, nil
}

func staticTokenSource(cred *clawyauth.AuthCredential) oauth2.TokenSource {
	return oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken:  cred.AccessToken,
		TokenType:    "Bearer",
		RefreshToken: cred.RefreshToken,
		Expiry:       cred.ExpiresAt,
	})
}

func generateOAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// fetchProtectedResourceMetadata tries the discovery locations mandated by
// the MCP authorization spec: the resource_metadata URL from the
// WWW-Authenticate challenge (when present), the well-known document next to
// the MCP endpoint path, and the well-known document at the origin root.
// Discovery is best-effort: (nil, "", nil) means "no metadata found".
func fetchProtectedResourceMetadata(
	ctx context.Context,
	wwwAuthenticate []string,
	serverURL string,
) (*oauthex.ProtectedResourceMetadata, string, error) {
	type candidate struct {
		metadataURL string
		resource    string
	}
	var candidates []candidate

	for _, challenge := range parseChallenges(wwwAuthenticate) {
		if !strings.EqualFold(challenge.Scheme, "bearer") {
			continue
		}
		if raw := strings.TrimSpace(challenge.Params["resource_metadata"]); raw != "" {
			candidates = append(candidates, candidate{metadataURL: raw, resource: serverURL})
		}
	}

	base, err := url.Parse(serverURL)
	if err != nil {
		return nil, "", fmt.Errorf("parsing server URL: %w", err)
	}

	pathWellKnown := *base
	pathWellKnown.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(base.Path, "/")
	candidates = append(candidates, candidate{metadataURL: pathWellKnown.String(), resource: serverURL})

	rootWellKnown := *base
	rootWellKnown.Path = "/.well-known/oauth-protected-resource"
	rootWellKnown.RawQuery, rootWellKnown.Fragment = "", ""
	candidates = append(candidates, candidate{metadataURL: rootWellKnown.String(), resource: stripPath(serverURL)})

	for _, cand := range candidates {
		prm, prmErr := oauthex.GetProtectedResourceMetadata(ctx, cand.metadataURL, cand.resource, oauthHTTPClient)
		if prmErr == nil && prm != nil && len(prm.AuthorizationServers) > 0 {
			return prm, strings.TrimRight(strings.TrimSpace(prm.AuthorizationServers[0]), "/"), nil
		}
	}
	return nil, "", nil
}

// fetchAuthServerMetadata retrieves authorization server metadata trying the
// OAuth (RFC 8414) and OpenID Connect discovery locations in order.
func fetchAuthServerMetadata(ctx context.Context, issuer string) (*oauthex.AuthServerMeta, error) {
	wellKnownPaths := []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/openid-configuration",
	}
	var lastErr error
	for _, suffix := range wellKnownPaths {
		meta, metaErr := oauthex.GetAuthServerMeta(ctx, issuer+suffix, issuer, oauthHTTPClient)
		if metaErr == nil && meta != nil {
			return meta, nil
		}
		if metaErr != nil {
			lastErr = metaErr
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("fetching authorization server metadata from %s: %w", issuer, lastErr)
	}
	return nil, fmt.Errorf("no authorization server metadata found at %s", issuer)
}

func parseChallenges(headers []string) []oauthex.Challenge {
	challenges, err := oauthex.ParseWWWAuthenticate(headers)
	if err != nil {
		return nil
	}
	return challenges
}

func stripPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Path, u.RawQuery, u.Fragment = "", "", ""
	return u.String()
}

// ── Interactive authorization ────────────────────────────────────────────────

type authorizationResult struct {
	Code  string
	State string
}

type waitForAuthorizationCodeOptions struct {
	authURL       string
	expectedState string
	redirectURI   string
	openBrowser   bool
	timeout       time.Duration
	stdin         io.Reader
	stdout        io.Writer
}

// waitForAuthorizationCode opens the browser, starts a localhost callback
// listener when applicable, and prompts the user to paste the full callback
// URL into the terminal. The first of {listener callback, pasted input} wins;
// both paths validate the OAuth state parameter.
func waitForAuthorizationCode(
	ctx context.Context,
	opts waitForAuthorizationCodeOptions,
) (*authorizationResult, error) {
	if opts.timeout <= 0 {
		opts.timeout = oauthLoginTimeout
	}
	loopback := isLoopbackRedirectURI(opts.redirectURI)

	if opts.openBrowser {
		if openErr := oauthOpenBrowser(opts.authURL); openErr != nil {
			fmt.Fprintf(opts.stdout,
				"Could not open the browser automatically (%v); open the URL above manually.\n", openErr)
		}
	} else {
		fmt.Fprintln(opts.stdout, "Automatic browser opening is disabled; open the URL above manually.")
	}

	codeCh := make(chan authorizationResult, 1)
	errCh := make(chan error, 1)

	var srv *http.Server
	if loopback {
		listener, port, listenErr := listenLoopback(callbackPortForRedirect(opts.redirectURI))
		switch {
		case listenErr != nil:
			fmt.Fprintf(opts.stdout,
				"note: could not bind local callback listener (%v); paste the callback URL below instead.\n",
				listenErr)
		default:
			srv = serveOAuthCallback(listener, opts.expectedState, codeCh, errCh)
			fmt.Fprintf(opts.stdout, "Waiting for the authorization callback on http://localhost:%d ...\n", port)
		}
	}

	canPaste := opts.stdin != nil && (oauthStdinIsTTY() || !isRealStdin(opts.stdin))
	if canPaste {
		fmt.Fprint(opts.stdout,
			"\nIf this machine cannot receive the localhost callback (headless, SSH, ...),\n"+
				"copy the FULL callback URL from your browser's address bar and paste it here,\n"+
				"(a bare authorization code works too), then press Enter:\n> ")
		go readPastedInput(opts.stdin, opts.expectedState, codeCh, errCh)
	} else if srv == nil {
		return nil, fmt.Errorf("no local callback listener possible and no interactive terminal to paste the callback URL")
	}

	defer func() {
		if srv != nil {
			shutdownServer(srv)
		}
	}()

	timeoutCh := time.After(opts.timeout)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return nil, fmt.Errorf("timed out waiting for authorization after %s", opts.timeout)
		case flowErr := <-errCh:
			return nil, flowErr
		case res := <-codeCh:
			return &res, nil
		}
	}
}

// serveOAuthCallback serves the redirect target until a valid callback or an
// error response arrives. Channels are buffered so handlers never block.
func serveOAuthCallback(
	listener net.Listener,
	expectedState string,
	codeCh chan<- authorizationResult,
	errCh chan<- error,
) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != expectedState {
			http.Error(w, "OAuth state mismatch", http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("OAuth state mismatch in callback"):
			default:
			}
			return
		}
		if errParam := q.Get("error"); errParam != "" {
			http.Error(w, "Authorization failed: "+errParam, http.StatusBadRequest)
			msg := "authorization server returned " + errParam
			if desc := q.Get("error_description"); desc != "" {
				msg += ": " + desc
			}
			select {
			case errCh <- fmt.Errorf("%s", msg):
			default:
			}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "No authorization code received", http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("callback contained no authorization code"):
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><h2>&#10003; Authentication successful</h2>"+
			"<p>You can close this window and return to Clawy.</p></body></html>")
		select {
		case codeCh <- authorizationResult{Code: code, State: expectedState}:
		default:
		}
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	return srv
}

// readPastedInput reads one pasted line and resolves it into an authorization
// result or an error.
func readPastedInput(r io.Reader, expectedState string, codeCh chan<- authorizationResult, errCh chan<- error) {
	reader := bufio.NewReader(r)
	line, readErr := reader.ReadString('\n')
	if strings.TrimSpace(line) == "" {
		switch {
		case readErr == io.EOF:
			select {
			case errCh <- fmt.Errorf("no callback URL was pasted"):
			default:
			}
		case readErr != nil:
			select {
			case errCh <- fmt.Errorf("reading pasted callback: %w", readErr):
			default:
			}
		}
		return
	}
	res, perr := parsePastedCallback(line, expectedState)
	if perr != nil {
		select {
		case errCh <- perr:
		default:
		}
		return
	}
	select {
	case codeCh <- *res:
	default:
	}
}

// parsePastedCallback extracts the authorization code from a pasted value.
// Accepted inputs: the full callback URL (…?code=…&state=…) or a bare
// authorization code. When expectedState is non-empty the state parameter
// must match it.
func parsePastedCallback(raw, expectedState string) (*authorizationResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "\"'`")
	if raw == "" {
		return nil, fmt.Errorf("empty callback input")
	}

	// Bare authorization code.
	if !strings.ContainsAny(raw, "?#/") {
		return &authorizationResult{Code: raw}, nil
	}

	parsed, err := url.Parse(raw)
	notURL := err != nil || (parsed.Scheme == "" && parsed.Host == "" && parsed.Opaque == "")
	if notURL {
		return &authorizationResult{Code: raw}, nil
	}

	query := parsed.Query()
	if errParam := query.Get("error"); errParam != "" {
		if desc := query.Get("error_description"); desc != "" {
			return nil, fmt.Errorf("authorization server returned %s: %s", errParam, desc)
		}
		return nil, fmt.Errorf("authorization server returned %s", errParam)
	}

	// Some providers deliver the code in the fragment (#code=...) — check both.
	params := query
	if query.Get("code") == "" && parsed.Fragment != "" {
		if frag, fragErr := url.ParseQuery(parsed.Fragment); fragErr == nil && frag.Get("code") != "" {
			params = frag
		}
	}

	code := params.Get("code")
	if code == "" {
		return nil, fmt.Errorf(
			"could not find an authorization code in the pasted input; paste the FULL callback URL including its code parameter")
	}
	state := params.Get("state")
	if expectedState != "" && state != expectedState {
		return nil, fmt.Errorf(
			"OAuth state mismatch: the pasted callback belongs to a different login attempt; restart authentication")
	}
	return &authorizationResult{Code: code, State: state}, nil
}

// isLoopbackRedirectURI reports whether the redirect URI points at this
// machine so a local listener can capture it.
func isLoopbackRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	switch strings.ToLower(host) {
	case "localhost":
		return true
	case "":
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// callbackPortForRedirect extracts the TCP port of a redirect URI.
func callbackPortForRedirect(raw string) int {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			return 443
		}
		return 80
	}
	n := 0
	for _, c := range port {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// listenLoopback binds a TCP listener on 127.0.0.1; port 0 selects an
// ephemeral port.
func listenLoopback(port int) (net.Listener, int, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, 0, err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return listener, addr.Port, nil
}

func shutdownServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// isRealStdin reports whether r is the process's actual standard input (as
// opposed to a test-injected reader).
func isRealStdin(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && f == os.Stdin
}

// ── Public entry points for the CLI ─────────────────────────────────────────

// LoginMCPServer runs the interactive OAuth flow for a configured MCP server
// and stores the resulting credential. It implements `clawy mcp auth`.
func LoginMCPServer(ctx context.Context, serverName string, cfg config.MCPServerConfig) error {
	handler, err := NewOAuthHandler(serverName, cfg)
	if err != nil {
		return err
	}
	if handler == nil {
		return fmt.Errorf("OAuth is not enabled for MCP server %q; add an oauth block or pass --oauth", serverName)
	}
	return handler.AuthorizeInteractive(ctx)
}

// LogoutMCPServer removes the stored OAuth credential for the named MCP
// server. Removing a missing credential is not an error.
func LogoutMCPServer(serverName string) error {
	if err := oauthCredStore.Delete(CredentialProviderName(serverName)); err != nil {
		return fmt.Errorf("removing stored credential for MCP server %q: %w", serverName, err)
	}
	return nil
}

// HasStoredMCPCredential reports whether a stored credential exists for the
// named MCP server.
func HasStoredMCPCredential(serverName string) bool {
	cred, err := oauthCredStore.Get(CredentialProviderName(serverName))
	return err == nil && cred != nil && cred.AccessToken != ""
}
