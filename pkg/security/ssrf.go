package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// SSRFProtection provides Server-Side Request Forgery protection.
type SSRFProtection struct {
	blockedSchemes   []string
	blockedHosts     map[string]struct{}
	blockedCIDRs     []*net.IPNet
	allowedSchemes   map[string]struct{}
	allowPrivate     bool
	dnsRebindGuard   bool
	resolvedCache    map[string][]net.IP
}

// NewSSRFProtection creates a new SSRF protection instance.
func NewSSRFProtection() *SSRFProtection {
	return &SSRFProtection{
		blockedSchemes: []string{"file", "gopher", "ftp", "dict", "ldap", "tftp"},
		blockedHosts:   defaultBlockedHosts(),
		allowedSchemes: map[string]struct{}{"http": {}, "https": {}},
		allowPrivate:   false,
		dnsRebindGuard: true,
		resolvedCache:  make(map[string][]net.IP),
	}
}

// WithAllowPrivate enables/disables private network access.
func (s *SSRFProtection) WithAllowPrivate(allow bool) *SSRFProtection {
	s.allowPrivate = allow
	return s
}

// WithBlockedSchemes sets the list of blocked URL schemes.
func (s *SSRFProtection) WithBlockedSchemes(schemes ...string) *SSRFProtection {
	s.blockedSchemes = schemes
	return s
}

// WithAllowedSchemes sets the list of allowed URL schemes.
func (s *SSRFProtection) WithAllowedSchemes(schemes ...string) *SSRFProtection {
	s.allowedSchemes = make(map[string]struct{})
	for _, scheme := range schemes {
		s.allowedSchemes[strings.ToLower(scheme)] = struct{}{}
	}
	return s
}

// ValidateURL checks if a URL is safe to fetch.
func (s *SSRFProtection) ValidateURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return &SecurityError{Category: "ssrf", Message: "empty URL"}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &SecurityError{Category: "ssrf", Message: "invalid URL: " + err.Error()}
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return &SecurityError{Category: "ssrf", Message: "missing URL scheme"}
	}

	// Check blocked schemes.
	for _, blocked := range s.blockedSchemes {
		if scheme == blocked {
			return &SecurityError{
				Category: "ssrf",
				Message:  "URL scheme '" + scheme + "' is not allowed",
			}
		}
	}

	// Check allowed schemes.
	if len(s.allowedSchemes) > 0 {
		if _, ok := s.allowedSchemes[scheme]; !ok {
			return &SecurityError{
				Category: "ssrf",
				Message:  "URL scheme '" + scheme + "' is not in allowed list",
			}
		}
	}

	host := parsed.Hostname()
	if host == "" {
		return &SecurityError{Category: "ssrf", Message: "missing URL host"}
	}

	// Check blocked hosts.
	if _, blocked := s.blockedHosts[strings.ToLower(host)]; blocked {
		return &SecurityError{
			Category: "ssrf",
			Message:  "host '" + host + "' is blocked",
		}
	}

	// Check for IP-based SSRF.
	if ip := net.ParseIP(host); ip != nil {
		if err := s.validateIP(ip); err != nil {
			return err
		}
	}

	// Block URLs with embedded credentials.
	if parsed.User != nil {
		return &SecurityError{
			Category: "ssrf",
			Message:  "URLs with embedded credentials are not allowed",
		}
	}

	// Block common SSRF bypass techniques.
	if strings.Contains(host, "@") {
		return &SecurityError{
			Category: "ssrf",
			Message:  "malformed host with '@' is not allowed",
		}
	}

	return nil
}

// ValidateIP checks if an IP address is safe to connect to.
func (s *SSRFProtection) ValidateIP(ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return &SecurityError{Category: "ssrf", Message: "invalid IP address"}
	}
	return s.validateIP(ip)
}

func (s *SSRFProtection) validateIP(ip net.IP) error {
	if !s.allowPrivate && isPrivateIP(ip) {
		return &SecurityError{
			Category: "ssrf",
			Message:  "private IP addresses are not allowed",
		}
	}
	for _, cidr := range s.blockedCIDRs {
		if cidr.Contains(ip) {
			return &SecurityError{
				Category: "ssrf",
				Message:  "IP " + ip.String() + " is in blocked range",
			}
		}
	}
	return nil
}

func defaultBlockedHosts() map[string]struct{} {
	hosts := map[string]struct{}{
		"localhost": {},
		"metadata.google.internal": {},
		"metadata.internal":        {},
		"169.254.169.254":          {}, // AWS metadata
		"100.100.100.200":          {}, // Alibaba cloud metadata
		"fd00:ec2::254":            {}, // AWS IPv6 metadata
	}
	return hosts
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"100.64.0.0/10",
		"198.18.0.0/15",
		"fc00::/7",
		"fe80::/10",
		"::1/128",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// SafeURL returns a sanitized version of the URL if it's safe.
func (s *SSRFProtection) SafeURL(rawURL string) (*url.URL, error) {
	if err := s.ValidateURL(rawURL); err != nil {
		return nil, err
	}
	return url.Parse(rawURL)
}

// AddBlockedHost adds a host to the blocklist.
func (s *SSRFProtection) AddBlockedHost(host string) {
	s.blockedHosts[strings.ToLower(strings.TrimSpace(host))] = struct{}{}
}

// AddBlockedCIDR adds a CIDR range to the blocklist.
func (s *SSRFProtection) AddBlockedCIDR(cidrStr string) error {
	_, network, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}
	s.blockedCIDRs = append(s.blockedCIDRs, network)
	return nil
}
