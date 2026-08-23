//go:build integration

package kit

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func IsolateProviderHomes(
	t *testing.T,
) {
	t.Helper()
	root := tempHome(t)
	t.Setenv("CODEX_HOME", isolatedCodexHome(t, root))
	t.Setenv("CLAUDE_CONFIG_DIR", isolatedClaudeHome(t, root))

	t.Setenv("CLAUDE_SECURESTORAGE_CONFIG_DIR", "")
}

func isolatedCodexHome(
	t *testing.T,
	root string,
) string {
	t.Helper()
	dir := filepath.Join(root, "codex")
	require.NoError(t, os.MkdirAll(dir, 0o700), "kit: create isolated CODEX_HOME")
	linkCodexAuth(t, dir)
	return dir
}

func linkCodexAuth(
	t *testing.T,
	dir string,
) {
	t.Helper()
	auth := filepath.Join(realUserHome(), ".codex", "auth.json")
	if _, err := os.Stat(auth); err != nil {
		return
	}
	require.NoError(t, os.Symlink(auth, filepath.Join(dir, "auth.json")),
		"kit: link codex auth into the isolated CODEX_HOME")
}

func isolatedClaudeHome(
	t *testing.T,
	root string,
) string {
	t.Helper()
	dir := filepath.Join(root, "claude")
	require.NoError(t, os.MkdirAll(dir, 0o700), "kit: create isolated CLAUDE_CONFIG_DIR")
	blob, err := json.Marshal(claudeSeed(t))
	require.NoError(t, err, "kit: marshal claude seed config")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude.json"), blob, 0o600),
		"kit: write claude seed config")
	return dir
}

func claudeSeed(
	t *testing.T,
) map[string]any {
	t.Helper()
	seed := map[string]any{"hasCompletedOnboarding": true}
	blob, err := os.ReadFile(filepath.Join(realUserHome(), ".claude.json"))
	if err == nil {
		_ = json.Unmarshal(blob, &seed)
	}
	seed["projects"] = map[string]any{}
	return seed
}

func realUserHome() string {
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
