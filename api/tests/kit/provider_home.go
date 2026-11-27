//go:build integration

package kit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	ClaudeCredentialsEnv = "CROWBAR_TEST_CLAUDE_CREDENTIALS"
	CodexAuthEnv         = "CROWBAR_TEST_CODEX_AUTH"
)

const (
	CodexHomeEnv  = "CODEX_HOME"
	ClaudeHomeEnv = "CLAUDE_CONFIG_DIR"
)

const claudeSeed = `{"hasCompletedOnboarding":true,"projects":{}}`

func IsolateProviderHomes(
	t *testing.T,
) string {
	t.Helper()
	root := tempHome(t)
	codex, claude := PointProviderHomesAt(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(claude, ".claude.json"), []byte(claudeSeed), 0o600),
		"kit: write claude seed config")
	copyCredential(t, CodexAuthEnv, filepath.Join(codex, "auth.json"))
	copyCredential(t, ClaudeCredentialsEnv, filepath.Join(claude, ".credentials.json"))
	return root
}

func PointProviderHomesAt(
	t *testing.T,
	root string,
) (codex, claude string) {
	t.Helper()
	codex, claude = filepath.Join(root, "codex"), filepath.Join(root, "claude")
	for env, home := range map[string]string{CodexHomeEnv: codex, ClaudeHomeEnv: claude} {
		require.NoError(t, os.MkdirAll(home, 0o700), "kit: create isolated provider home")
		t.Setenv(env, home)
	}
	return codex, claude
}

func RequireProviderHomeIn(
	t *testing.T,
	env string,
	root string,
) string {
	t.Helper()
	dir, err := ProviderHomeIn(env, root)
	require.NoError(t, err)
	return dir
}

func ProviderHomeIn(
	env string,
	root string,
) (string, error) {
	named := os.Getenv(env)
	if named == "" {
		return "", fmt.Errorf("kit: %s is unset", env)
	}
	dir, err := filepath.EvalSymlinks(named)
	if err != nil {
		return "", fmt.Errorf("kit: resolve %s: %w", env, err)
	}
	rootDir, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("kit: resolve the test's temp dir: %w", err)
	}
	if !within(dir, rootDir) {
		return "", fmt.Errorf("kit: %s is %s, outside the test's temp dir %s", env, named, root)
	}
	return dir, nil
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
	require.NoError(t, outsideProviderHomes(env, src))
	blob, err := os.ReadFile(src) //nolint:gosec // G304: the path is the one the test env var names
	require.NoError(t, err, "kit: read the credential %s names", env)
	require.NoError(t, os.WriteFile(dst, blob, 0o600), "kit: copy the credential %s names", env) //nolint:gosec // G703: dst is the kit's own path under the test's temp home
}

func outsideProviderHomes(
	env string,
	path string,
) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("kit: locate the user's home: %w", err)
	}
	named, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("kit: resolve %s: %w", env, err)
	}
	if inProviderHome(home, named) {
		return fmt.Errorf("kit: %s names %s, inside a provider's own home: copy it elsewhere first", env, path)
	}
	resolved, err := filepath.EvalSymlinks(named)
	if err != nil {
		return fmt.Errorf("kit: resolve %s: %w", env, err)
	}
	if inProviderHome(home, resolved) {
		return fmt.Errorf("kit: %s links %s into a provider's own home: copy it elsewhere first", env, path)
	}
	return nil
}

func inProviderHome(home, path string) bool {
	for _, own := range []string{".claude", ".codex", ".claude.json"} {
		if within(path, filepath.Join(home, own)) {
			return true
		}
	}
	return false
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}
