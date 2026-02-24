package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

// WebTool supports fetch operations.
// Args: {"url": "https://..."}

const (
	defaultWebTimeout      = 15 * time.Second
	defaultWebMaxBodyBytes = int64(1 << 20) // 1 MiB
	maxWebRedirects        = 10
)

var blockedIPv4Prefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // shared address space (CGNAT)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved + limited broadcast
}

var blockedIPv6Prefixes = []netip.Prefix{
	netip.MustParsePrefix("100::/64"),      // discard-only
	netip.MustParsePrefix("2001:2::/48"),   // benchmarking
	netip.MustParsePrefix("2001:db8::/32"), // documentation
}

type WebTool struct {
	client       *http.Client
	maxBodyBytes int64
	allowPrivate bool
}

func NewWebTool() *WebTool {
	allowPrivate := envTrue("PICOBOT_WEB_ALLOW_PRIVATE")
	return newWebToolWithOptions(allowPrivate, defaultWebMaxBodyBytes)
}

func newWebToolWithOptions(allowPrivate bool, maxBodyBytes int64) *WebTool {
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultWebMaxBodyBytes
	}

	t := &WebTool{
		maxBodyBytes: maxBodyBytes,
		allowPrivate: allowPrivate,
	}

	transport := &http.Transport{
		Proxy:       nil,
		DialContext: t.dialContext,
	}

	t.client = &http.Client{
		Timeout:   defaultWebTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxWebRedirects {
				return fmt.Errorf("web: too many redirects")
			}
			return t.validateTarget(req.URL)
		},
	}

	return t
}

func (t *WebTool) Name() string        { return "web" }
func (t *WebTool) Description() string { return "Fetch web content from a URL" }

func (t *WebTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch (must be http or https)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	u, ok := args["url"].(string)
	if !ok || u == "" {
		return "", fmt.Errorf("web: 'url' argument required")
	}

	parsed, err := parseWebURL(u)
	if err != nil {
		return "", err
	}
	if err := t.validateTarget(parsed); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "picobot-web-tool/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := readLimitedString(resp.Body, 4096)
		if msg != "" {
			return "", fmt.Errorf("web: request failed with status %s: %s", resp.Status, msg)
		}
		return "", fmt.Errorf("web: request failed with status %s", resp.Status)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, t.maxBodyBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(b)) > t.maxBodyBytes {
		return "", fmt.Errorf("web: response body exceeds %d bytes limit", t.maxBodyBytes)
	}

	return string(b), nil
}

func parseWebURL(raw string) (*url.URL, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("web: invalid URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return nil, fmt.Errorf("web: unsupported URL scheme %q (allowed: http, https)", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("web: URL host is required")
	}
	return u, nil
}

func (t *WebTool) validateTarget(u *url.URL) error {
	if t.allowPrivate {
		return nil
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("web: URL host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("web: private/special-use targets are blocked")
	}
	if ip, err := netip.ParseAddr(host); err == nil && isPrivateOrLocalIP(ip) {
		return fmt.Errorf("web: private/special-use targets are blocked")
	}
	return nil
}

func (t *WebTool) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: defaultWebTimeout}
	if t.allowPrivate {
		return dialer.DialContext(ctx, network, addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	host = strings.TrimSpace(host)
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		return nil, fmt.Errorf("web: private/special-use targets are blocked")
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		if isPrivateOrLocalIP(ip) {
			return nil, fmt.Errorf("web: private/special-use targets are blocked")
		}
		return dialer.DialContext(ctx, network, addr)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("web: DNS lookup failed for %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("web: no IP addresses found for host %q", host)
	}

	var lastErr error
	publicFound := false
	for _, ipa := range ips {
		ip, ok := netip.AddrFromSlice(ipa.IP)
		if !ok {
			continue
		}
		ip = ip.Unmap()
		if isPrivateOrLocalIP(ip) {
			continue
		}
		publicFound = true

		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}

	if !publicFound {
		return nil, fmt.Errorf("web: host %q resolves only to private/special-use addresses", host)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("web: unable to connect to host %q", host)
}

func isPrivateOrLocalIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}

	if ip.Is4() {
		return containsPrefix(ip, blockedIPv4Prefixes)
	}
	if ip.Is6() {
		return containsPrefix(ip, blockedIPv6Prefixes)
	}
	return false
}

func containsPrefix(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func envTrue(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func readLimitedString(r io.Reader, limit int64) (string, error) {
	if limit <= 0 {
		return "", nil
	}
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
