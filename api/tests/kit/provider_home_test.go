//go:build integration

package kit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolateProviderHomes_CodexRunsAuthenticatedOutOfTheTempHome(t *testing.T) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex not installed")
	}
	if os.Getenv(CodexAuthEnv) == "" {
		t.Skipf("set %s to a codex auth.json kept outside ~/.codex", CodexAuthEnv)
	}

	IsolateProviderHomes(t)

	cmd := exec.Command(codex, "login", "status")
	cmd.Env = os.Environ()
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "codex login status: %s", out)
	assert.NotContains(t, strings.ToLower(string(out)), "not logged in",
		"the copied auth.json did not carry the credential into the isolated CODEX_HOME")
}

func TestIsolateProviderHomes_ClaudeConfigIsSeededWithNoPlaces(t *testing.T) {
	IsolateProviderHomes(t)

	blob, err := os.ReadFile(filepath.Join(os.Getenv(ClaudeHomeEnv), ".claude.json"))
	require.NoError(t, err, "the isolated config dir must be seeded before any CLI reads it")
	var seeded map[string]any
	require.NoError(t, json.Unmarshal(blob, &seeded))
	assert.Equal(t, map[string]any{}, seeded["projects"],
		"the seed must start with NO trusted places, so the first claude still shows its trust dialog")
	assert.Equal(t, true, seeded["hasCompletedOnboarding"],
		"without this the CLI opens on its theme picker instead of the trust dialog")
}

// A credential file is copied in, never linked: a CLI that rewrites its own
// credential must not write through to the file the test was handed.
func TestIsolateProviderHomes_CopiesTheNamedCredentialFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"token":"t"}`), 0o600))
	t.Setenv(CodexAuthEnv, src)
	t.Setenv(ClaudeCredentialsEnv, "")

	IsolateProviderHomes(t)

	dst := filepath.Join(os.Getenv(CodexHomeEnv), "auth.json")
	info, err := os.Lstat(dst)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "the credential must be a copy, not a link")
	blob, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"t"}`, string(blob))
	assert.NoFileExists(t, filepath.Join(os.Getenv(ClaudeHomeEnv), ".credentials.json"),
		"no credential is named for claude, so none is written")
}

// A helper writes or deletes under a provider home only when the env var
// resolves inside the test's temp dir; a link out of it does not count.
func TestProviderHomeIn_RefusesAHomeOutsideTheTestsTempDir(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	inside := filepath.Join(root, "codex")
	require.NoError(t, os.Mkdir(inside, 0o700))
	link := filepath.Join(root, "linked")
	require.NoError(t, os.Symlink(elsewhere, link))

	for name, tc := range map[string]struct {
		home string
		ok   bool
	}{
		"inside":                   {inside, true},
		"outside":                  {elsewhere, false},
		"a link inside to outside": {link, false},
		"unset":                    {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(CodexHomeEnv, tc.home)
			dir, err := ProviderHomeIn(CodexHomeEnv, root)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			want, err := filepath.EvalSymlinks(inside)
			require.NoError(t, err)
			assert.Equal(t, want, dir)
		})
	}
}

// A credential in a provider's own home is refused, named directly or through
// a link to it.
func TestIsolateProviderHomes_RefusesACredentialInAProviderHome(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))
	inClaude := filepath.Join(home, ".claude", ".credentials.json")
	require.NoError(t, os.WriteFile(inClaude, []byte("{}"), 0o600))
	link := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, os.Symlink(inClaude, link))
	elsewhere := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(elsewhere, []byte("{}"), 0o600))

	require.Error(t, outsideProviderHomes(ClaudeCredentialsEnv, inClaude), "named inside ~/.claude")
	require.Error(t, outsideProviderHomes(ClaudeCredentialsEnv, link), "a link into ~/.claude")
	require.NoError(t, outsideProviderHomes(CodexAuthEnv, elsewhere))
}
