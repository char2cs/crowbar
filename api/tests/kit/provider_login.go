//go:build integration

package kit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// RequireProviderLogin skips a test that needs a real model turn from
// provider when this machine holds no credential for it. Without one the CLI
// parks on its login screen and the test would wait out its whole budget.
func RequireProviderLogin(
	t *testing.T,
	provider string,
) {
	t.Helper()
	if !providerLoggedIn(provider) {
		t.Skipf("no %s credentials on this machine: this test needs a real model turn", provider)
	}
}

func providerLoggedIn(
	provider string,
) bool {
	home := realUserHome()
	switch provider {
	case "claude":
		return os.Getenv("ANTHROPIC_API_KEY") != "" ||
			fileExists(filepath.Join(home, ".claude", ".credentials.json")) ||
			(runtime.GOOS == "darwin" && keychainHasItem("Claude Code-credentials"))
	case "codex":
		return os.Getenv("OPENAI_API_KEY") != "" || fileExists(filepath.Join(home, ".codex", "auth.json"))
	default:
		return false
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// keychainHasItem reports whether the macOS login keychain holds service.
func keychainHasItem(
	service string,
) bool {
	return exec.Command("security", "find-generic-password", "-s", service).Run() == nil //nolint:gosec // fixed service names
}
