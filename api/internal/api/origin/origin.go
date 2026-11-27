package origin

import (
	"net/netip"
	"net/url"
	"os"
	"strings"
)

const EnvAllowedOrigins = "CROWBAR_ALLOWED_ORIGINS"

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
