package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/config"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"v0.2.0", "v0.3.0", true},
		{"v0.3.0", "v0.3.0", false},
		{"v0.3.0", "v0.2.0", false},
		{"0.2.0", "0.2.1", true},
		{"v1.2", "v1.2.1", true},
		{"v1.10.0", "v1.9.0", false},
		{"v0.3.0-nightly.20240101.abc123", "v0.3.0", false}, // same numeric core
		{"dev", "v0.3.0", false},
		{"unknown", "v0.3.0", false},
		{"", "v0.3.0", false},
		{"v0.3.0", "", false},
		{"v0.3.0-dirty", "v0.4.0", false},
	}
	for _, tc := range tests {
		if got := IsNewerVersion(tc.current, tc.latest); got != tc.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestGetLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v0.9.9",
			"name":         "v0.9.9",
			"html_url":     "https://github.com/Kantemba/clawy/releases/tag/v0.9.9",
			"prerelease":   false,
			"published_at": "2026-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	withTestHTTPClient(t, server.Client())

	info, err := GetLatestRelease(server.URL + "/releases/latest")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if info.TagName != "v0.9.9" {
		t.Fatalf("tag = %q, want v0.9.9", info.TagName)
	}
}

func TestFormatUpdateNotice(t *testing.T) {
	st := UpdateStatus{Current: "v0.2.0", Latest: ReleaseInfo{TagName: "v0.3.0", HTMLURL: "https://example.com/r"}, UpdateAvailable: true}
	notice := FormatUpdateNotice(st)
	if notice == "" {
		t.Fatal("expected notice, got empty")
	}
	st.UpdateAvailable = false
	if got := FormatUpdateNotice(st); got != "" {
		t.Fatalf("expected empty notice, got %q", got)
	}
}

func TestUpdateCheckCacheRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAWY_HOME", home)

	oldVersion := config.Version
	config.Version = "v0.2.0"
	defer func() { config.Version = oldVersion }()

	st := UpdateStatus{
		Current:         "v0.2.0",
		Latest:          ReleaseInfo{TagName: "v0.3.0"},
		UpdateAvailable: true,
		CheckedAt:       time.Now().UTC(),
	}
	saveCachedStatus(st)

	// Cache file should exist under CLAWY_HOME.
	if _, err := os.Stat(filepath.Join(home, UpdateCheckFile)); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	loaded, ok := loadCachedStatus(UpdateCheckInterval)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !loaded.UpdateAvailable || loaded.Latest.TagName != "v0.3.0" {
		t.Fatalf("unexpected cached status: %+v", loaded)
	}

	// Stale cache should miss when maxAge is tiny and timestamp is old.
	st.CheckedAt = time.Now().UTC().Add(-48 * time.Hour)
	saveCachedStatus(st)
	if _, ok := loadCachedStatus(UpdateCheckInterval); ok {
		t.Fatal("expected stale cache miss")
	}
}
