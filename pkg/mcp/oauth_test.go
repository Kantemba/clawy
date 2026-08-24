package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	clawyauth "github.com/Kantemba/clawy/pkg/auth"
	"github.com/Kantemba/clawy/pkg/config"
)

// memCredStore is an in-memory credentialStore for tests.
type memCredStore struct {
	mu  sync.Mutex
	mem map[string]*clawyauth.AuthCredential
}

func newMemCredStore() *memCredStore {
	return &memCredStore{mem: map[string]*clawyauth.AuthCredential{}}
}

func (m *memCredStore) Get(provider string) (*clawyauth.AuthCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cred := m.mem[provider]
	if cred == nil {
		return nil, nil
	}
	cp := *cred
	return &cp, nil
}

func (m *memCredStore) Set(provider string, cred *clawyauth.AuthCredential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *cred
	m.mem[provider] = &cp
	return nil
}

func (m *memCredStore) Delete(provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mem, provider)
	return nil
}

// useTestOAuthEnv swaps the injectable OAuth dependencies for the duration of
// the test and restores them afterwards.
func useTestOAuthEnv(t *testing.T, stdin io.Reader, stdout io.Writer) *memCredStore {
	t.Helper()
	store := newMemCredStore()
	origStore, origStdin, origStdout := oauthCredStore, oauthStdin, oauthStdout
	origTTY, origBrowser, origClient := oauthStdinIsTTY, oauthOpenBrowser, oauthHTTPClient
	oauthCredStore = store
	oauthStdin = stdin
	oauthStdout = stdout
	oauthStdinIsTTY = func() bool { return true }
	oauthOpenBrowser = func(string) error { return nil }
	oauthHTTPClient = &http.Client{Timeout: 10 * time.Second}
	t.Cleanup(func() {
		oauthCredStore, oauthStdin, oauthStdout = origStore, origStdin, origStdout
		oauthStdinIsTTY, oauthOpenBrowser, oauthHTTPClient = origTTY, origBrowser, origClient
	})
	return store
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocating free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestParsePastedCallback(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		state    string
		wantCode string
		wantErr  bool
	}{
		{name: "full url", raw: "http://localhost:19877/callback?code=abc&state=s1", state: "s1", wantCode: "abc"},
		{name: "quoted full url", raw: `"http://localhost/cb?code=abc&state=s1"`, state: "s1", wantCode: "abc"},
		{name: "bare code", raw: "abc123", wantCode: "abc123"},
		{name: "fragment code", raw: "http://localhost/cb#code=frag1&state=s1", state: "s1", wantCode: "frag1"},
		{name: "state mismatch", raw: "http://localhost/cb?code=abc&state=other", state: "s1", wantErr: true},
		{name: "missing code", raw: "http://localhost/cb?state=s1", state: "s1", wantErr: true},
		{name: "error response", raw: "http://localhost/cb?error=access_denied&error_description=nope", wantErr: true},
		{name: "empty input", raw: "   ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parsePastedCallback(tt.raw, tt.state)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", res)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", res.Code, tt.wantCode)
			}
		})
	}
}

func TestIsLoopbackRedirectURI(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:19877/callback": true,
		"http://127.0.0.1:9999/cb":        true,
		"http://[::1]:9000/cb":            true,
		"http://example.com/cb":           false,
		"https://clawy.io/oauth":          false,
		"not a url":                       false,
	}
	for raw, want := range cases {
		if got := isLoopbackRedirectURI(raw); got != want {
			t.Errorf("isLoopbackRedirectURI(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestCallbackPortForRedirect(t *testing.T) {
	cases := map[string]int{
		"http://localhost:19877/callback": 19877,
		"http://localhost/callback":       80,
		"https://localhost/cb":            443,
	}
	for raw, want := range cases {
		if got := callbackPortForRedirect(raw); got != want {
			t.Errorf("callbackPortForRedirect(%q) = %d, want %d", raw, got, want)
		}
	}
}

// authServerCapture records requests received by the mock authorization
// server so tests can assert protocol details.
type authServerCapture struct {
	mu           sync.Mutex
	redirectURIs []string
	tokenForms   []url.Values
}

func (c *authServerCapture) setRedirectURIs(uris []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.redirectURIs = uris
}

func (c *authServerCapture) getRedirectURIs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.redirectURIs...)
}

func (c *authServerCapture) addTokenForm(form url.Values) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokenForms = append(c.tokenForms, form)
}

func (c *authServerCapture) getTokenForms() []url.Values {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]url.Values(nil), c.tokenForms...)
}

// startMockOAuthStack serves protected resource metadata, authorization
// server metadata, dynamic client registration, the token endpoint, and a
// protected /mcp endpoint that answers 401 with a bearer challenge.
func startMockOAuthStack(t *testing.T) (*httptest.Server, *authServerCapture) {
	t.Helper()
	capture := &authServerCapture{}
	var baseURL string

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	asm := func() map[string]any {
		return map[string]any{
			"issuer":                                baseURL,
			"authorization_endpoint":                baseURL + "/authorize",
			"token_endpoint":                        baseURL + "/token",
			"registration_endpoint":                 baseURL + "/register",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, asm())
	})
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, asm())
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"resource": baseURL + "/mcp", "authorization_servers": []string{baseURL}})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var meta struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		_ = json.NewDecoder(r.Body).Decode(&meta)
		capture.setRedirectURIs(meta.RedirectURIs)
		writeJSON(w, map[string]any{
			"client_id":                  "dyn-client",
			"redirect_uris":              meta.RedirectURIs,
			"token_endpoint_auth_method": "none",
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		capture.addTokenForm(r.PostForm)
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			if r.PostForm.Get("code") != "good-code" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			if r.PostForm.Get("code_verifier") == "" ||
				r.PostForm.Get("resource") == "" ||
				r.PostForm.Get("client_id") == "" {
				http.Error(w, `{"error":"invalid_request","error_description":"missing pkce/resource/client"}`, http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"access_token":  "at-123",
				"refresh_token": "rt-456",
				"expires_in":    3600,
				"scope":         "read write",
			})
		case "refresh_token":
			if r.PostForm.Get("refresh_token") == "" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"access_token": "at-refreshed", "expires_in": 3600})
		default:
			http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate",
			`Bearer resource_metadata="`+baseURL+`/.well-known/oauth-protected-resource/mcp"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	srv := httptest.NewServer(mux)
	baseURL = srv.URL
	t.Cleanup(srv.Close)
	return srv, capture
}

// TestAuthorizeFlowWithPastedCallback drives the full interactive flow using
// a scripted terminal paste of the full callback URL, exercising discovery,
// dynamic client registration, PKCE exchange and persistence.
func TestAuthorizeFlowWithPastedCallback(t *testing.T) {
	stack, capture := startMockOAuthStack(t)

	port := freeTCPPort(t)
	handler, herr := NewOAuthHandler("test", config.MCPServerConfig{
		URL: stack.URL + "/mcp",
		OAuth: &config.MCPOAuthConfig{
			Scopes:       []string{"read", "write"},
			CallbackPort: port,
		},
	})
	if herr != nil {
		t.Fatalf("NewOAuthHandler: %v", herr)
	}

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	store := useTestOAuthEnv(t, stdinR, stdoutW)
	defer func() { _ = stdinW.Close() }()
	defer func() { _ = stdoutW.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Drain stdout continuously; when the authorization URL appears, simulate
	// the user pasting the full callback URL from a separate goroutine so the
	// producing side is never blocked by this consumer.
	go func() {
		scanner := bufio.NewScanner(stdoutR)
		pasted := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if pasted {
				continue
			}
			authURL, perr := url.Parse(line)
			if perr != nil || authURL.Query().Get("state") == "" || authURL.Query().Get("client_id") == "" {
				continue
			}
			pasted = true
			callback := fmt.Sprintf("http://localhost:%d/callback?code=good-code&state=%s",
				port, authURL.Query().Get("state"))
			go func() {
				defer func() { _ = recover() }() // write may race test cleanup
				_, _ = io.WriteString(stdinW, callback+"\n")
			}()
		}
	}()

	if aerr := handler.AuthorizeInteractive(ctx); aerr != nil {
		t.Fatalf("AuthorizeInteractive: %v", aerr)
	}

	cred, gerr := store.Get(CredentialProviderName("test"))
	if gerr != nil || cred == nil {
		t.Fatalf("stored credential missing: %v", gerr)
	}
	if cred.AccessToken != "at-123" || cred.RefreshToken != "rt-456" {
		t.Fatalf("unexpected tokens in credential: %+v", cred)
	}
	if cred.ClientID != "dyn-client" {
		t.Fatalf("client id = %q, want dyn-client", cred.ClientID)
	}
	if cred.TokenEndpoint != stack.URL+"/token" {
		t.Fatalf("token endpoint = %q", cred.TokenEndpoint)
	}

	wantRedirect := fmt.Sprintf("http://localhost:%d/callback", port)
	registered := false
	for _, uri := range capture.getRedirectURIs() {
		if uri == wantRedirect {
			registered = true
		}
	}
	if !registered {
		t.Fatalf("redirect %q not registered; got %v", wantRedirect, capture.getRedirectURIs())
	}

	var codeForm url.Values
	for _, form := range capture.getTokenForms() {
		if form.Get("grant_type") == "authorization_code" {
			codeForm = form
		}
	}
	if codeForm == nil {
		t.Fatal("no authorization_code token request captured")
	}
	if codeForm.Get("client_id") != "dyn-client" {
		t.Fatalf("token client_id = %q", codeForm.Get("client_id"))
	}
	if codeForm.Get("resource") != stack.URL+"/mcp" {
		t.Fatalf("token resource = %q", codeForm.Get("resource"))
	}

	ts, terr := handler.TokenSource(ctx)
	if terr != nil || ts == nil {
		t.Fatalf("TokenSource = %v, %v", ts, terr)
	}
	tok, terr := ts.Token()
	if terr != nil || tok.AccessToken != "at-123" {
		t.Fatalf("token = %+v, err %v", tok, terr)
	}
}

// TestTokenSourceSilentRefresh verifies an expired stored credential is
// transparently refreshed via the token endpoint and re-persisted.
func TestTokenSourceSilentRefresh(t *testing.T) {
	stack, _ := startMockOAuthStack(t)
	store := useTestOAuthEnv(t, nil, &strings.Builder{})

	provider := CredentialProviderName("srv")
	if serr := store.Set(provider, &clawyauth.AuthCredential{
		AccessToken:   "stale",
		RefreshToken:  "rt-old",
		ExpiresAt:     time.Now().Add(-time.Hour),
		Provider:      provider,
		ClientID:      "dyn-client",
		TokenEndpoint: stack.URL + "/token",
	}); serr != nil {
		t.Fatalf("seeding store: %v", serr)
	}

	handler, herr := NewOAuthHandler("srv", config.MCPServerConfig{
		URL:   stack.URL + "/mcp",
		OAuth: &config.MCPOAuthConfig{},
	})
	if herr != nil {
		t.Fatalf("NewOAuthHandler: %v", herr)
	}

	ctx := context.Background()
	ts, terr := handler.TokenSource(ctx)
	if terr != nil || ts == nil {
		t.Fatalf("TokenSource = %v, %v", ts, terr)
	}
	tok, terr := ts.Token()
	if terr != nil {
		t.Fatalf("Token: %v", terr)
	}
	if tok.AccessToken != "at-refreshed" {
		t.Fatalf("access token = %q, want at-refreshed", tok.AccessToken)
	}

	updated, gerr := store.Get(provider)
	if gerr != nil || updated == nil || updated.AccessToken != "at-refreshed" {
		t.Fatalf("refreshed credential not persisted: %+v (%v)", updated, gerr)
	}
	if updated.ExpiresAt.Before(time.Now()) {
		t.Fatal("persisted credential is still expired")
	}
}

// TestConnectServerUsesStoredOAuthToken connects to a guarded streamable MCP
// server using a pre-seeded OAuth credential.
func TestConnectServerUsesStoredOAuthToken(t *testing.T) {
	store := useTestOAuthEnv(t, nil, &strings.Builder{})
	provider := CredentialProviderName("guarded")
	if serr := store.Set(provider, &clawyauth.AuthCredential{
		AccessToken: "tok-fixed",
		ExpiresAt:   time.Now().Add(time.Hour),
		Provider:    provider,
		AuthMethod:  "oauth",
	}); serr != nil {
		t.Fatalf("seeding store: %v", serr)
	}

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "guarded-mcp", Version: "1.0.0"}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{Name: "ping", Description: "ping"},
		func(context.Context, *sdkmcp.CallToolRequest, map[string]any) (*sdkmcp.CallToolResult, any, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "pong"}}}, nil, nil
		})
	streamable := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil)
	guarded := http.NewServeMux()
	guarded.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-fixed" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(guarded)
	t.Cleanup(srv.Close)

	mgr := NewManager()
	defer func() { _ = mgr.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfgSrv := config.MCPServerConfig{
		Enabled: true,
		Type:    "http",
		URL:     srv.URL + "/mcp",
		OAuth:   &config.MCPOAuthConfig{},
	}
	if cerr := mgr.ConnectServer(ctx, "guarded", cfgSrv); cerr != nil {
		t.Fatalf("ConnectServer: %v", cerr)
	}
	conn, ok := mgr.GetServer("guarded")
	if !ok || len(conn.Tools) == 0 {
		t.Fatalf("expected connected server with tools, ok=%v tools=%d", ok, len(conn.Tools))
	}
}


