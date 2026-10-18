//go:build integration

package kit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The env vars a real-provider test takes a credential file from. The file is
// copied into the isolated home, never linked, so a CLI cannot write back to it.
const (
	ClaudeCredentialsEnv = "CROWBAR_TEST_CLAUDE_CREDENTIALS"
	CodexAuthEnv         = "CROWBAR_TEST_CODEX_AUTH"
)

// claudeSeed answers claude's first-run onboarding and trusts no place.
const claudeSeed = `{"hasCompletedOnboarding":true,"projects":{}}`

// IsolateProviderHomes points codex's and claude's own homes at throwaway
// directories. It reads nothing of the user's: a credential is only ever the
// file a test env var names.
func IsolateProviderHomes(
	t *testing.T,
) {
	t.Helper()
	root := tempHome(t)
	codex, claude := filepath.Join(root, "codex"), filepath.Join(root, "claude")
	for _, dir := range []string{codex, claude} {
		require.NoError(t, os.MkdirAll(dir, 0o700), "kit: create isolated provider home")
	}
	t.Setenv("CODEX_HOME", codex)
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	require.NoError(t, os.WriteFile(filepath.Join(claude, ".claude.json"), []byte(claudeSeed), 0o600),
		"kit: write claude seed config")
	copyCredential(t, CodexAuthEnv, filepath.Join(codex, "auth.json"))
	copyCredential(t, ClaudeCredentialsEnv, filepath.Join(claude, ".credentials.json"))
}

func copyCredential(
	t *testing.T,
	env string,
	dst string,
) {
	t.Helper()
	src := os.Getenv(env)
	if src == "" {
		return
	}
	requireOutsideProviderHomes(t, env, src)
	blob, err := os.ReadFile(src) //nolint:gosec // G304: the path is the one the test env var names
	require.NoError(t, err, "kit: read the credential %s names", env)
	require.NoError(t, os.WriteFile(dst, blob, 0o600), "kit: copy the credential %s names", env)
}

// requireOutsideProviderHomes refuses a credential kept in a provider's own
// home, which the kit never reads.
func requireOutsideProviderHomes(
	t *testing.T,
	env string,
	path string,
) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err, "kit: locate the user's home")
	abs, err := filepath.Abs(path)
	require.NoError(t, err, "kit: resolve %s", env)
	for _, own := range []string{".claude", ".codex", ".claude.json"} {
		rel, err := filepath.Rel(filepath.Join(home, own), abs)
		if err == nil && !strings.HasPrefix(rel, "..") {
			t.Fatalf("kit: %s names %s, inside a provider's own home: copy it elsewhere first", env, path)
		}
	}
}
