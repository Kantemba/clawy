package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Kantemba/clawy/pkg/config"
)

// ReleaseInfo describes a GitHub release used for update notifications.
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
}

// UpdateStatus is the result of an update check.
type UpdateStatus struct {
	Current         string      `json:"current"`
	Latest          ReleaseInfo `json:"latest"`
	UpdateAvailable bool        `json:"update_available"`
	CheckedAt       time.Time   `json:"checked_at"`
	Cached          bool        `json:"cached,omitempty"`
}

const (
	// UpdateCheckInterval controls how often background checks hit the network.
	UpdateCheckInterval = 24 * time.Hour
	// UpdateCheckFile is the cache file name under $CLAWY_HOME.
	UpdateCheckFile = ".update-check.json"
	// EnvNoUpdateCheck disables background update notifications when set.
	EnvNoUpdateCheck = "CLAWY_NO_UPDATE_CHECK"
	// EnvUpdateRepo overrides the owner/repo used for update checks
	// (e.g. "myfork/clawy" for forks). Default: "Kantemba/clawy".
	EnvUpdateRepo = "CLAWY_UPDATE_REPO"
)

// GetLatestRelease queries the GitHub Releases API for the latest stable release.
func GetLatestRelease(apiURL string) (ReleaseInfo, error) {
	if apiURL == "" {
		apiURL = releaseAPIURL()
	}
	resp, err := getWithRetry(apiURL)
	if err != nil {
		return ReleaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ReleaseInfo{}, fmt.Errorf("failed to query releases: status %d", resp.StatusCode)
	}
	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ReleaseInfo{}, err
	}
	if info.TagName == "" {
		return ReleaseInfo{}, fmt.Errorf("release API returned empty tag_name")
	}
	return info, nil
}

// releaseAPIURL returns the releases/latest API URL, honouring CLAWY_UPDATE_REPO.
func releaseAPIURL() string {
	if v := strings.TrimSpace(os.Getenv(EnvUpdateRepo)); v != "" {
		v = strings.Trim(v, "/")
		return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", v)
	}
	return GetProdReleaseAPIURL()
}

// IsNewerVersion reports whether latest is newer than current.
// Unknown/dev builds never count as outdated (returns false).
func IsNewerVersion(current, latest string) bool {
	c := normalizeVersion(current)
	l := normalizeVersion(latest)
	if c == "" || l == "" {
		return false
	}
	if c == l {
		return false
	}
	cParts := versionParts(c)
	lParts := versionParts(l)
	for i := 0; i < len(cParts) && i < len(lParts); i++ {
		if lParts[i] != cParts[i] {
			return lParts[i] > cParts[i]
		}
	}
	// Longer version with same prefix (e.g. 1.2 vs 1.2.1) is newer.
	return len(lParts) > len(cParts)
}

// normalizeVersion strips v-prefix, build metadata (+...), pre-release suffix
// comparison noise and dirty markers. Returns "" for non-comparable versions.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	// Dev / unknown / dirty worktree builds can't be compared reliably.
	// A dirty suffix (e.g. "v0.3.0-dirty") means local changes; don't nag.
	if lower == "dev" || lower == "unknown" || strings.Contains(lower, "dirty") {
		return ""
	}
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	// Strip build metadata (+sha) — keep pre-release core for numeric compare.
	if i := strings.Index(v, "+"); i >= 0 {
		v = v[:i]
	}
	// Strip pre-release suffix (-nightly..., -rc.1) for numeric core compare,
	// but remember it: a stable release is newer than its own pre-release.
	base := v
	pre := ""
	if i := strings.Index(v, "-"); i >= 0 {
		base = v[:i]
		pre = v[i+1:]
		_ = pre
	}
	if base == "" {
		return ""
	}
	return base
}

// versionParts parses "1.2.3" into [1 2 3]. Non-numeric segments become 0.
func versionParts(v string) []int {
	segs := strings.Split(v, ".")
	out := make([]int, 0, len(segs))
	for _, s := range segs {
		// Strip any trailing non-digit run (e.g. "3rc1" -> "3").
		digits := ""
		for _, r := range s {
			if r >= '0' && r <= '9' {
				digits += string(r)
			} else {
				break
			}
		}
		if digits == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

// CheckForUpdate fetches the latest release and compares it to currentVersion.
// If currentVersion is empty, config.Version is used.
func CheckForUpdate(currentVersion string) (UpdateStatus, error) {
	if currentVersion == "" {
		currentVersion = config.GetVersion()
	}
	info, err := GetLatestRelease("")
	if err != nil {
		return UpdateStatus{}, err
	}
	return UpdateStatus{
		Current:         currentVersion,
		Latest:          info,
		UpdateAvailable: IsNewerVersion(currentVersion, info.TagName),
		CheckedAt:       time.Now().UTC(),
	}, nil
}

// FormatUpdateNotice returns a one-line user-facing notice, or "" when up to date.
func FormatUpdateNotice(st UpdateStatus) string {
	if !st.UpdateAvailable {
		return ""
	}
	where := st.Latest.HTMLURL
	if where == "" {
		where = "https://github.com/Kantemba/clawy/releases/latest"
	}
	return fmt.Sprintf("Update available: %s → %s  %s\nRun `clawy update` to upgrade.",
		displayVersion(st.Current), displayVersion(st.Latest.TagName), where)
}

func displayVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "dev"
	}
	return v
}

// updateCheckCachePath returns the cache file path.
func updateCheckCachePath() string {
	return filepath.Join(config.GetHome(), UpdateCheckFile)
}

func loadCachedStatus(maxAge time.Duration) (UpdateStatus, bool) {
	bs, err := os.ReadFile(updateCheckCachePath())
	if err != nil {
		return UpdateStatus{}, false
	}
	var st UpdateStatus
	if err := json.Unmarshal(bs, &st); err != nil {
		return UpdateStatus{}, false
	}
	if st.Latest.TagName == "" || st.CheckedAt.IsZero() {
		return UpdateStatus{}, false
	}
	if maxAge > 0 && time.Since(st.CheckedAt) > maxAge {
		return UpdateStatus{}, false
	}
	st.Cached = true
	// Re-evaluate against the running binary (cache may be from older binary).
	current := config.GetVersion()
	st.Current = current
	st.UpdateAvailable = IsNewerVersion(current, st.Latest.TagName)
	return st, true
}

func saveCachedStatus(st UpdateStatus) {
	st.Cached = false
	bs, err := json.Marshal(st)
	if err != nil {
		return
	}
	path := updateCheckCachePath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, bs, 0o644)
}

// CachedUpdateNotice returns a formatted notice from the local cache only
// (no network). Use on CLI startup so notifications never block.
func CachedUpdateNotice() (string, bool) {
	if os.Getenv(EnvNoUpdateCheck) != "" {
		return "", false
	}
	st, ok := loadCachedStatus(UpdateCheckInterval)
	if !ok || !st.UpdateAvailable {
		return "", false
	}
	return FormatUpdateNotice(st), true
}

// RefreshUpdateCache fetches the latest release and refreshes the cache.
// Network errors are swallowed; this is safe to run in a background goroutine.
func RefreshUpdateCache() {
	if os.Getenv(EnvNoUpdateCheck) != "" {
		return
	}
	st, err := CheckForUpdate("")
	if err != nil {
		return
	}
	saveCachedStatus(st)
}

// MaybeCheckForUpdate performs a cached background check.
// It returns ("", false, nil) when checks are disabled, the cache is fresh
// with no update, or the network check fails (never blocks startup).
func MaybeCheckForUpdate() (string, bool) {
	if os.Getenv(EnvNoUpdateCheck) != "" {
		return "", false
	}
	if st, ok := loadCachedStatus(UpdateCheckInterval); ok {
		if !st.UpdateAvailable {
			return "", false
		}
		return FormatUpdateNotice(st), true
	}
	st, err := CheckForUpdate("")
	if err != nil {
		return "", false
	}
	saveCachedStatus(st)
	if !st.UpdateAvailable {
		return "", false
	}
	return FormatUpdateNotice(st), true
}
