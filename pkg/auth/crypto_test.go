package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptBlob(t *testing.T) {
	passphrase := "test-passphrase-123"
	plaintext := []byte(`{"credentials":{"test":{"access_token":"secret"}}}`)

	encrypted, err := encryptBlob(plaintext, passphrase)
	if err != nil {
		t.Fatalf("encryptBlob failed: %v", err)
	}

	if string(encrypted) == string(plaintext) {
		t.Error("encrypted data should differ from plaintext")
	}

	decrypted, err := decryptBlob(encrypted, passphrase)
	if err != nil {
		t.Fatalf("decryptBlob failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	plaintext := []byte("secret data")
	encrypted, _ := encryptBlob(plaintext, "correct-passphrase")

	_, err := decryptBlob(encrypted, "wrong-passphrase")
	if err == nil {
		t.Error("expected error with wrong passphrase")
	}
}

func TestEncryptedAuthStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	passphrase := "test-secret-key"

	store := NewEncryptedAuthStore(path, func() string { return passphrase })

	// Add a credential.
	err := store.SetCredential("test-provider", &AuthCredential{
		AccessToken: "secret-access-token",
		RefreshToken: "secret-refresh-token",
		Provider:    "test-provider",
	})
	if err != nil {
		t.Fatalf("SetCredential failed: %v", err)
	}

	// Save.
	if err := store.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file is encrypted (not plaintext).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) == "" {
		t.Error("file should not be empty")
	}
	// Should not contain plaintext token.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["credentials"]; ok {
			t.Error("file should not contain plaintext credentials")
		}
	}

	// Load in a new store.
	store2 := NewEncryptedAuthStore(path, func() string { return passphrase })
	if err := store2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cred := store2.GetCredential("test-provider")
	if cred == nil {
		t.Fatal("GetCredential returned nil")
	}
	if cred.AccessToken != "secret-access-token" {
		t.Errorf("AccessToken = %s, want secret-access-token", cred.AccessToken)
	}
	if cred.RefreshToken != "secret-refresh-token" {
		t.Errorf("RefreshToken = %s, want secret-refresh-token", cred.RefreshToken)
	}
}

func TestEncryptedAuthStoreWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	store := NewEncryptedAuthStore(path, func() string { return "correct" })
	_ = store.SetCredential("test", &AuthCredential{AccessToken: "secret"})
	_ = store.Save()

	store2 := NewEncryptedAuthStore(path, func() string { return "wrong" })
	err := store2.Load()
	if err == nil {
		t.Error("expected error loading with wrong passphrase")
	}
}

func TestEncryptedAuthStoreMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// Write a legacy plaintext store.
	legacy := `{"credentials":{"test":{"access_token":"legacy-token","provider":"test"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Load with encrypted store — should migrate.
	store := NewEncryptedAuthStore(path, func() string { return "new-passphrase" })
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cred := store.GetCredential("test")
	if cred == nil || cred.AccessToken != "legacy-token" {
		t.Error("legacy credential not loaded")
	}

	// Save should now encrypt.
	if err := store.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
}
