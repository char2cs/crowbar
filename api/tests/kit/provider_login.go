//go:build integration

package kit

import (
	"os"
	"testing"
)

// RequireProviderLogin skips a test that needs a real model turn from
// provider unless a test env var supplies its credential: an API key, or a
// credential file IsolateProviderHomes copies in.
func RequireProviderLogin(
	t *testing.T,
	provider string,
) {
	t.Helper()
	keyEnv, fileEnv := credentialEnv(provider)
	if keyEnv == "" {
		t.Skipf("no credential source for %s", provider)
	}
	if os.Getenv(keyEnv) == "" && os.Getenv(fileEnv) == "" {
		t.Skipf("set %s or %s: this test needs a real %s model turn", keyEnv, fileEnv, provider)
	}
}

func credentialEnv(
	provider string,
) (key, file string) {
	switch provider {
	case "claude":
		return "ANTHROPIC_API_KEY", ClaudeCredentialsEnv
	case "codex":
		return "OPENAI_API_KEY", CodexAuthEnv
	default:
		return "", ""
	}
}
