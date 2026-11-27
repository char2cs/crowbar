//go:build integration

package kit

import (
	"os"
	"testing"
)

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
