package security

import (
	"testing"
)

func TestSSRFProtection(t *testing.T) {
	ssrf := NewSSRFProtection()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com/api/data", false},
		{"valid http", "http://example.com/api", false},
		{"empty URL", "", true},
		{"file scheme", "file:///etc/passwd", true},
		{"gopher scheme", "gopher://example.com", true},
		{"localhost", "http://localhost:8080", true},
		{"127.0.0.1", "http://127.0.0.1:3000", true},
		{"10.x", "http://10.0.0.1/internal", true},
		{"192.168.x", "http://192.168.1.1/admin", true},
		{"AWS metadata", "http://169.254.169.254/latest/meta-data", true},
		{"with credentials", "http://user:pass@example.com", true},
		{"at sign in host", "http://evil@google.com", true},
		{"missing host", "http://", true},
		{"missing scheme", "example.com/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ssrf.ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestSSRFAllowPrivate(t *testing.T) {
	ssrf := NewSSRFProtection().WithAllowPrivate(true)
	err := ssrf.ValidateURL("http://10.0.0.1/api")
	if err != nil {
		t.Errorf("expected nil error with AllowPrivate, got %v", err)
	}
}

func TestSSRFBlockedHost(t *testing.T) {
	ssrf := NewSSRFProtection()
	ssrf.AddBlockedHost("evil.com")
	err := ssrf.ValidateURL("https://evil.com/steal")
	if err == nil {
		t.Error("expected error for blocked host")
	}
}

func TestSSRFBlockedCIDR(t *testing.T) {
	ssrf := NewSSRFProtection()
	if err := ssrf.AddBlockedCIDR("203.0.113.0/24"); err != nil {
		t.Fatalf("AddBlockedCIDR failed: %v", err)
	}
	err := ssrf.ValidateURL("https://203.0.113.5/api")
	if err == nil {
		t.Error("expected error for blocked CIDR")
	}
}

func TestSSRFIPValidation(t *testing.T) {
	ssrf := NewSSRFProtection()
	err := ssrf.ValidateIP("127.0.0.1")
	if err == nil {
		t.Error("expected error for loopback IP")
	}
	err = ssrf.ValidateIP("8.8.8.8")
	if err != nil {
		t.Errorf("expected nil for public IP, got %v", err)
	}
}
