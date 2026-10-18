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

// The env vars a real-provider test takes a credential file from. The file is
// copied into the isolated home, never linked, so a CLI cannot write back to it.
const (
	ClaudeCredentialsEnv = "CROWBAR_TEST_CLAUDE_CREDENTIALS"
	CodexAuthEnv         = "CROWBAR_TEST_CODEX_AUTH"
)

// The env vars that name codex's and claude's own homes.
const (
	CodexHomeEnv  = "CODEX_HOME"
	ClaudeHomeEnv = "CLAUDE_CONFIG_DIR"
)

// claudeSeed answers claude's first-run onboarding and trusts no place.
const claudeSeed = `{"hasCompletedOnboarding":true,"projects":{}}`

// IsolateProviderHomes points codex's and claude's own homes at throwaway
// directories and returns their root. It reads nothing of the user's: a
// credential is only ever the file a test env var names.
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

// PointProviderHomesAt points codex's and claude's own homes at fresh, empty
// directories under root, a temp dir the test owns, and returns them.
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

// RequireProviderHomeIn is ProviderHomeIn for a helper holding t: it fails the
// test rather than write or delete outside root.
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

// ProviderHomeIn is the directory env names, resolved through any symlink,
// and an error unless it lies inside root. Whatever writes or deletes under a
// provider home asks this first, so it never reaches a real one.
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
	require.NoError(t, os.WriteFile(dst, blob, 0o600), "kit: copy the credential %s names", env)
}

// outsideProviderHomes refuses a credential kept in a provider's own home,
// which the kit never reads. Both sides are resolved, so a link cannot hide one.
func outsideProviderHomes(
	env string,
	path string,
) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("kit: locate the user's home: %w", err)
	}
	cred, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("kit: resolve %s: %w", env, err)
	}
	for _, own := range []string{".claude", ".codex", ".claude.json"} {
		for _, guarded := range resolvedForms(filepath.Join(home, own)) {
			if within(cred, guarded) {
				return fmt.Errorf("kit: %s names %s, inside a provider's own home: copy it elsewhere first", env, path)
			}
		}
	}
	return nil
}

// resolvedForms is path as written and as its symlinks resolve, when they do.
func resolvedForms(path string) []string {
	forms := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		forms = append(forms, resolved)
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		forms = append(forms, filepath.Join(dir, filepath.Base(path)))
	}
	return forms
}

// within reports whether path is root or lies under it.
func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}
