package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/credential"
	"github.com/Kantemba/clawy/pkg/fileutil"
)

// EncryptedAuthStore wraps AuthStore with encryption at rest.
type EncryptedAuthStore struct {
	mu        sync.Mutex
	store     *AuthStore
	path      string
	passphrase func() string
}

// authStoreData is the on-disk format for encrypted auth data.
type authStoreData struct {
	Encrypted bool              `json:"encrypted"`
	Data      string            `json:"data,omitempty"` // base64-encoded encrypted blob
	Credentials map[string]*AuthCredential `json:"credentials,omitempty"` // plaintext (legacy/migration)
}

// NewEncryptedAuthStore creates an encrypted auth store.
func NewEncryptedAuthStore(path string, passphrase func() string) *EncryptedAuthStore {
	return &EncryptedAuthStore{
		store:      &AuthStore{Credentials: make(map[string]*AuthCredential)},
		path:       path,
		passphrase: passphrase,
	}
}

// Load reads and decrypts the auth store from disk.
func (e *EncryptedAuthStore) Load() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	data, err := os.ReadFile(e.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read auth store: %w", err)
	}

	var fileData authStoreData
	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("parse auth store: %w", err)
	}

	// Legacy plaintext format — migrate to encrypted on next save.
	if !fileData.Encrypted {
		e.store.Credentials = fileData.Credentials
		if e.store.Credentials == nil {
			e.store.Credentials = make(map[string]*AuthCredential)
		}
		return nil
	}

	if fileData.Data == "" {
		return nil
	}

	// Decrypt.
	plaintext, err := decryptBlob(fileData.Data, e.passphrase())
	if err != nil {
		return fmt.Errorf("decrypt auth store: %w", err)
	}

	var store AuthStore
	if err := json.Unmarshal(plaintext, &store); err != nil {
		return fmt.Errorf("parse decrypted auth store: %w", err)
	}
	e.store = &store
	return nil
}

// Save encrypts and writes the auth store to disk.
func (e *EncryptedAuthStore) Save() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.passphrase == nil || e.passphrase() == "" {
		// No passphrase available — store plaintext with a warning.
		return e.savePlaintext()
	}

	// Serialize.
	plaintext, err := json.Marshal(e.store)
	if err != nil {
		return fmt.Errorf("marshal auth store: %w", err)
	}

	// Encrypt.
	encrypted, err := encryptBlob(plaintext, e.passphrase())
	if err != nil {
		return fmt.Errorf("encrypt auth store: %w", err)
	}

	fileData := authStoreData{
		Encrypted: true,
		Data:      encrypted,
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal encrypted store: %w", err)
	}

	return fileutil.WriteFileAtomic(e.path, data, 0o600)
}

func (e *EncryptedAuthStore) savePlaintext() error {
	fileData := authStoreData{
		Encrypted:   false,
		Credentials: e.store.Credentials,
	}
	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plaintext store: %w", err)
	}
	return fileutil.WriteFileAtomic(e.path, data, 0o600)
}

// GetCredential retrieves a credential by provider.
func (e *EncryptedAuthStore) GetCredential(provider string) *AuthCredential {
	e.mu.Lock()
	defer e.mu.Unlock()
	cred, ok := e.store.Credentials[canonicalProvider(provider)]
	if !ok {
		return nil
	}
	return cloneCredential(cred)
}

// SetCredential stores a credential.
func (e *EncryptedAuthStore) SetCredential(provider string, cred *AuthCredential) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	canonical := canonicalProvider(provider)
	normalized := cloneCredential(cred)
	if normalized != nil {
		normalized.Provider = canonicalProvider(normalized.Provider)
		if normalized.Provider == "" {
			normalized.Provider = canonical
		}
	}
	e.store.Credentials[canonical] = normalized
	return nil
}

// DeleteCredential removes a credential.
func (e *EncryptedAuthStore) DeleteCredential(provider string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.store.Credentials, canonicalProvider(provider))
}

// PassphraseProviderFunc returns the credential passphrase provider.
func PassphraseProviderFunc() func() string {
	return credential.PassphraseProvider
}

// encryptBlob encrypts data using AES-256-GCM with a passphrase-derived key.
func encryptBlob(plaintext []byte, passphrase string) (string, error) {
	if passphrase == "" {
		return "", fmt.Errorf("empty passphrase")
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key := deriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Prepend salt to ciphertext.
	result := append(salt, ciphertext...)
	return base64.StdEncoding.EncodeToString(result), nil
}

// decryptBlob decrypts data encrypted with encryptBlob.
func decryptBlob(encoded string, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("empty passphrase")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if len(data) < 16 {
		return nil, fmt.Errorf("data too short")
	}

	salt := data[:16]
	ciphertext := data[16:]

	key := deriveKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// deriveKey derives a 256-bit key from passphrase and salt using SHA-256.
func deriveKey(passphrase string, salt []byte) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(passphrase))
	return h.Sum(nil)
}

// AuthStorePath returns the path to the auth store file.
func AuthStorePath() string {
	return filepath.Join(config.GetHome(), "auth.json")
}
