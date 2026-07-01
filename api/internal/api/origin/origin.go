// Package origin decides whether a browser Origin may talk to the daemon. It is
// a leaf package (stdlib only) so the CORS middleware and both WebSocket
// upgraders can share one allowlist without an import cycle.
package origin

import (
	"net/url"
	"os"
	"strings"
)

// EnvAllowedOrigins is the env var holding extra exact-match allowed origins
// (comma-separated). It is the production safety valve for an app origin that is
// neither loopback nor the tauri:// scheme, so a platform whose webview origin we
// did not anticipate can be allowed without a code change.
const EnvAllowedOrigins = "CROWBAR_ALLOWED_ORIGINS"

// Allowed reports whether a browser Origin may call the daemon. The daemon's
// production transport is a unix socket with no remote reach, so this primarily
// guards the dev TCP path and the in-app webview against a malicious website
// driving the API through the user's browser (CSRF / cross-origin WS hijack).
//
// requestHost is the request's Host header (host[:port]); it enables the
// same-origin allowance for the case where the daemon serves its own SPA.
//
// Allowed:
//   - empty Origin: non-browser clients (curl, native, the Tauri IPC bridge) send
//     no Origin and are not subject to the same-origin policy.
//   - the tauri:// custom scheme, plus localhost and any *.localhost host — the
//     Tauri v2 webview origin differs by platform (tauri://localhost on macOS,
//     http://tauri.localhost elsewhere).
//   - loopback hosts (127.0.0.0/8, ::1), any scheme/port — local dev servers
//     (Vite) and the daemon serving its own SPA over loopback.
//   - same-origin: the Origin authority equals the request Host.
//   - exact matches from CROWBAR_ALLOWED_ORIGINS.
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

// isLoopbackHost reports whether host is an IPv4/IPv6 loopback literal. The whole
// 127.0.0.0/8 block is loopback, so any 127.* literal qualifies.
func isLoopbackHost(host string) bool {
	if host == "::1" {
		return true
	}
	return strings.HasPrefix(host, "127.")
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
