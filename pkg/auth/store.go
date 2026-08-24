package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/fileutil"
)

type AuthCredential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Provider     string    `json:"provider"`
	AuthMethod   string    `json:"auth_method"`
	Email        string    `json:"email,omitempty"`
	ProjectID    string    `json:"project_id,omitempty"`
	// OAuth client registration metadata. These fields are populated for
	// credentials obtained through generic OAuth 2.0 flows (e.g. remote MCP
	// servers) so that tokens can be refreshed and clients re-used without
	// re-registering. All fields are optional.
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret string   `json:"client_secret,omitempty"`
	TokenEndpoint string  `json:"token_endpoint,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

type AuthStore struct {
	Credentials map[string]*AuthCredential `json:"credentials"`
}

const (
	providerGoogleAntigravity = "google-antigravity"
	providerAntigravityAlias  = "antigravity"
)

func (c *AuthCredential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

func (c *AuthCredential) NeedsRefresh() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(5 * time.Minute).After(c.ExpiresAt)
}

func authFilePath() string {
	return filepath.Join(config.GetHome(), "auth.json")
}

func canonicalProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case providerAntigravityAlias:
		return providerGoogleAntigravity
	default:
		return normalized
	}
}

func cloneCredential(cred *AuthCredential) *AuthCredential {
	if cred == nil {
		return nil
	}
	cp := *cred
	return &cp
}

func mergeCredentials(primary, secondary *AuthCredential) *AuthCredential {
	if primary == nil {
		return cloneCredential(secondary)
	}

	merged := *primary
	if secondary == nil {
		return &merged
	}
	if merged.AccessToken == "" {
		merged.AccessToken = secondary.AccessToken
	}
	if merged.RefreshToken == "" {
		merged.RefreshToken = secondary.RefreshToken
	}
	if merged.AccountID == "" {
		merged.AccountID = secondary.AccountID
	}
	if merged.ExpiresAt.IsZero() {
		merged.ExpiresAt = secondary.ExpiresAt
	}
	if merged.Provider == "" {
		merged.Provider = secondary.Provider
	}
	if merged.AuthMethod == "" {
		merged.AuthMethod = secondary.AuthMethod
	}
	if merged.Email == "" {
		merged.Email = secondary.Email
	}
	if merged.ProjectID == "" {
		merged.ProjectID = secondary.ProjectID
	}
	if merged.ClientID == "" {
		merged.ClientID = secondary.ClientID
	}
	if merged.ClientSecret == "" {
		merged.ClientSecret = secondary.ClientSecret
	}
	if merged.TokenEndpoint == "" {
		merged.TokenEndpoint = secondary.TokenEndpoint
	}
	if len(merged.Scopes) == 0 {
		merged.Scopes = secondary.Scopes
	}

	return &merged
}

func shouldPreferCredential(
	candidate *AuthCredential,
	candidateCanonical bool,
	current *AuthCredential,
	currentCanonical bool,
) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}

	switch {
	case candidate.ExpiresAt.After(current.ExpiresAt):
		return true
	case current.ExpiresAt.After(candidate.ExpiresAt):
		return false
	case candidateCanonical != currentCanonical:
		return candidateCanonical
	default:
		return false
	}
}

func normalizeStore(store *AuthStore) {
	if store == nil {
		return
	}
	if store.Credentials == nil {
		store.Credentials = make(map[string]*AuthCredential)
		return
	}

	normalized := make(map[string]*AuthCredential, len(store.Credentials))
	canonicalFlags := make(map[string]bool, len(store.Credentials))

	for provider, cred := range store.Credentials {
		normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
		canonical := canonicalProvider(provider)
		normalizedCred := cloneCredential(cred)
		if normalizedCred != nil {
			normalizedCred.Provider = canonicalProvider(normalizedCred.Provider)
			if normalizedCred.Provider == "" {
				normalizedCred.Provider = canonical
			}
		}

		current := normalized[canonical]
		currentCanonical := canonicalFlags[canonical]
		candidateCanonical := normalizedProvider == canonical

		if shouldPreferCredential(normalizedCred, candidateCanonical, current, currentCanonical) {
			normalized[canonical] = mergeCredentials(normalizedCred, current)
			canonicalFlags[canonical] = candidateCanonical
			continue
		}

		normalized[canonical] = mergeCredentials(current, normalizedCred)
	}

	store.Credentials = normalized
}

func LoadStore() (*AuthStore, error) {
	path := authFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AuthStore{Credentials: make(map[string]*AuthCredential)}, nil
		}
		return nil, err
	}

	var store AuthStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	normalizeStore(&store)
	return &store, nil
}

func SaveStore(store *AuthStore) error {
	path := authFilePath()
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func GetCredential(provider string) (*AuthCredential, error) {
	store, err := LoadStore()
	if err != nil {
		return nil, err
	}
	cred, ok := store.Credentials[canonicalProvider(provider)]
	if !ok {
		return nil, nil
	}
	return cred, nil
}

func SetCredential(provider string, cred *AuthCredential) error {
	store, err := LoadStore()
	if err != nil {
		return err
	}

	canonical := canonicalProvider(provider)
	normalized := cloneCredential(cred)
	if normalized != nil {
		normalized.Provider = canonicalProvider(normalized.Provider)
		if normalized.Provider == "" {
			normalized.Provider = canonical
		}
	}

	store.Credentials[canonical] = normalized
	return SaveStore(store)
}

func DeleteCredential(provider string) error {
	store, err := LoadStore()
	if err != nil {
		return err
	}
	delete(store.Credentials, canonicalProvider(provider))
	return SaveStore(store)
}

func DeleteAllCredentials() error {
	path := authFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
