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

	blob, err := os.ReadFile(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".claude.json"))
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

	dst := filepath.Join(os.Getenv("CODEX_HOME"), "auth.json")
	info, err := os.Lstat(dst)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "the credential must be a copy, not a link")
	blob, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"t"}`, string(blob))
	assert.NoFileExists(t, filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".credentials.json"),
		"no credential is named for claude, so none is written")
}
