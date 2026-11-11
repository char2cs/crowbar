// Package origin decides whether a browser Origin may talk to the daemon. It is
// a leaf package (stdlib only) so the CORS middleware and both WebSocket
// upgraders can share one allowlist without an import cycle.
package origin

import (
	"net/netip"
	"net/url"
	"os"
	"strings"
)

// EnvAllowedOrigins is the env var holding extra comma-separated exact-match origins,
// so an unanticipated webview origin can be allowed without a code change.
const EnvAllowedOrigins = "CROWBAR_ALLOWED_ORIGINS"

// Allowed reports whether a browser Origin may call the daemon (CSRF / cross-origin
// WS hijack guard). It admits no Origin (non-browser clients), tauri:// and *.localhost,
// loopback hosts, same-origin with requestHost, and CROWBAR_ALLOWED_ORIGINS entries.
func Allowed(origin, requestHost string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" && u.Scheme == "" {
		return false
	}
	if u.Scheme == "tauri" {
		return true
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || isLoopbackHost(host) {
		return true
	}
	if requestHost != "" && u.Host == requestHost {
		return true
	}
	for _, allowed := range parseList(os.Getenv(EnvAllowedOrigins)) {
		if allowed == origin {
			return true
		}
	}
	return false
}

// isLoopbackHost reports whether host is an IPv4/IPv6 loopback literal
// (127.0.0.0/8, ::1). A DNS name never is, whatever it starts with.
func isLoopbackHost(host string) bool {
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func parseList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
