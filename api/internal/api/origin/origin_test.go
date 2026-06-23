package origin

import "testing"

func TestAllowed(t *testing.T) {
	const reqHost = "127.0.0.1:3737"
	cases := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"empty origin (non-browser client)", "", reqHost, true},
		{"tauri scheme macOS webview", "tauri://localhost", reqHost, true},
		{"tauri.localhost http webview", "http://tauri.localhost", reqHost, true},
		{"vite dev server localhost", "http://localhost:5173", reqHost, true},
		{"loopback ipv4 any port", "http://127.0.0.1:9000", reqHost, true},
		{"loopback ipv4 block", "http://127.0.0.2:8080", reqHost, true},
		{"loopback ipv6", "http://[::1]:5173", reqHost, true},
		{"https localhost", "https://localhost", reqHost, true},
		{"same-origin as request host", "http://127.0.0.1:3737", reqHost, true},
		{"malicious external site", "http://evil.com", reqHost, false},
		{"https external site", "https://evil.example.org", reqHost, false},
		{"lookalike subdomain of evil", "http://localhost.evil.com", reqHost, false},
		{"non-loopback lan ip", "http://192.168.1.50:3737", reqHost, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allowed(tc.origin, tc.host); got != tc.want {
				t.Fatalf("Allowed(%q, %q) = %v, want %v", tc.origin, tc.host, got, tc.want)
			}
		})
	}
}

// TestAllowed_EnvAllowlist proves the CROWBAR_ALLOWED_ORIGINS safety valve adds
// exact-match production origins without a code change.
func TestAllowed_EnvAllowlist(t *testing.T) {
	const prodOrigin = "https://app.crowbar.example"
	if Allowed(prodOrigin, "") {
		t.Fatalf("expected %q rejected before env allowlist set", prodOrigin)
	}
	t.Setenv(EnvAllowedOrigins, "https://other.example, "+prodOrigin)
	if !Allowed(prodOrigin, "") {
		t.Fatalf("expected %q allowed via CROWBAR_ALLOWED_ORIGINS", prodOrigin)
	}
	if Allowed("https://not-listed.example", "") {
		t.Fatalf("origin not in the env allowlist must stay rejected")
	}
}
