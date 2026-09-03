//go:build integration

package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolateProviderHomes_CodexRunsAuthenticatedOutOfTheTempHome(t *testing.T) {
	codex := lookupCLI(t, "codex")
	requireRealCodexLogin(t)

	before := SnapshotProviderHomes()
	IsolateProviderHomes(t)

	home := os.Getenv("CODEX_HOME")
	require.NotEmpty(t, home, "IsolateProviderHomes must set CODEX_HOME")
	require.NotEqual(t, filepath.Join(realUserHome(), ".codex"), home,
		"the isolated CODEX_HOME must not BE the user's real codex home")

	out := runCLI(t, codex, "login", "status")
	require.Contains(t, strings.ToLower(out), "logged in",
		"codex is not authenticated inside the isolated CODEX_HOME, so every agent test "+
			"would fail on auth rather than on its own assertion. Output: %q", out)
	require.NotContains(t, strings.ToLower(out), "not logged in",
		"codex reports NOT logged in inside the isolated CODEX_HOME: linking auth.json "+
			"did not carry the credential. Output: %q", out)

	require.Empty(t, SnapshotProviderHomes().Added(before),
		"running codex under an isolated CODEX_HOME still wrote a place into the user's real home")
}

func TestIsolateProviderHomes_ClaudeKeychainLookupStaysOnTheRealCredential(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("claude stores credentials in the macOS keychain only")
	}
	if !keychainHasItem(t, "Claude Code-credentials") {
		t.Skip("no claude login on this machine")
	}

	IsolateProviderHomes(t)

	value, ok := os.LookupEnv("CLAUDE_SECURESTORAGE_CONFIG_DIR")
	require.True(t, ok, "CLAUDE_SECURESTORAGE_CONFIG_DIR must be SET; unset is what suffixes the keychain name")
	require.Equal(t, "", value, "it must be set to the EMPTY string; any other value suffixes the name differently")
	require.Contains(t, os.Environ(), "CLAUDE_SECURESTORAGE_CONFIG_DIR=",
		"the empty value has to survive into the child's environment as \"KEY=\", or the CLI reads it as unset")

	assert.True(t, keychainHasItem(t, "Claude Code-credentials"),
		"the credential claude will look up must exist")
	assert.False(t, keychainHasItem(t, suffixedKeychainService(os.Getenv("CLAUDE_CONFIG_DIR"))),
		"the SUFFIXED name has an item, so this test can no longer tell the two lookups apart")
}

func TestIsolateProviderHomes_ClaudeConfigIsSeededWithNoPlaces(t *testing.T) {
	IsolateProviderHomes(t)

	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	require.NotEmpty(t, dir, "IsolateProviderHomes must set CLAUDE_CONFIG_DIR")
	require.NotEqual(t, filepath.Join(realUserHome(), ".claude"), dir,
		"the isolated CLAUDE_CONFIG_DIR must not BE the user's real claude home")

	blob, err := os.ReadFile(filepath.Join(dir, ".claude.json"))
	require.NoError(t, err, "the isolated config dir must be seeded before any CLI reads it")

	var seeded map[string]any
	require.NoError(t, json.Unmarshal(blob, &seeded))
	assert.Equal(t, map[string]any{}, seeded["projects"],
		"the seed must start with NO trusted places, so the first claude still shows its trust dialog")
	assert.Equal(t, true, seeded["hasCompletedOnboarding"],
		"without this the CLI opens on its theme picker instead of the trust dialog")

	real := realClaudeConfig(t)
	if real == nil {
		return
	}
	for _, flag := range oneTimeModalFlags {
		if want, ok := real[flag]; ok {
			assert.Equal(t, want, seeded[flag],
				"%q is a one-time-modal flag the real config has already answered; dropping it "+
					"puts an unexpected dialog in front of the harness", flag)
		}
	}
}

func TestProviderHomeRecord_AddedNamesEveryNewPlace(t *testing.T) {
	dir := t.TempDir()
	codex := filepath.Join(dir, "config.toml")
	claude := filepath.Join(dir, ".claude.json")

	writeFile(t, codex, codexTrustStanzas("/repo/kept"))
	writeFile(t, claude, `{"projects":{"/repo/kept":{}},"numStartups":1}`)
	before := ProviderHomeRecord{codexProjectKeys(codex), claudeProjectKeys(claude)}

	writeFile(t, codex, codexTrustStanzas("/repo/kept", "/private/tmp/leaked-repo"))
	writeFile(t, claude, `{"projects":{"/repo/kept":{},"/private/tmp/leaked-chat":{}},"numStartups":2}`)
	after := ProviderHomeRecord{codexProjectKeys(codex), claudeProjectKeys(claude)}

	assert.Empty(t, before.Added(before), "a home compared with itself has leaked nothing")
	assert.Empty(t, before.Added(after), "places going AWAY is the user pruning their own config, not a leak")

	report := after.Added(before)
	require.NotEmpty(t, report, "two new places were added and the guard reported nothing")
	assert.Contains(t, report, "/private/tmp/leaked-repo", "the leaked codex place must be named")
	assert.Contains(t, report, "/private/tmp/leaked-chat", "the leaked claude place must be named")
	assert.NotContains(t, report, "/repo/kept", "a place that was already there is not a leak")
	assert.NotContains(t, report, "numStartups",
		"the guard counts PLACES; a provider rewriting its own counters is not pollution")
}

func TestSnapshotProviderHomes_SurvivesAbsentHomes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	assert.Empty(t, codexProjectKeys(missing))
	assert.Empty(t, claudeProjectKeys(missing))

	snap := SnapshotProviderHomes()
	assert.Empty(t, snap.Added(snap))
}

var oneTimeModalFlags = []string{
	"hasCompletedOnboarding",
	"hasCompletedClaudeInChromeOnboarding",
	"theme",
}

func codexTrustStanzas(
	places ...string,
) string {
	var out strings.Builder
	for i, place := range places {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString("[projects.\"" + place + "\"]\ntrust_level = \"trusted\"\n")
	}
	return out.String()
}

func realClaudeConfig(
	t *testing.T,
) map[string]any {
	t.Helper()
	blob, err := os.ReadFile(filepath.Join(realUserHome(), ".claude.json"))
	if err != nil {
		return nil
	}
	var real map[string]any
	if err := json.Unmarshal(blob, &real); err != nil {
		return nil
	}
	return real
}

func requireRealCodexLogin(
	t *testing.T,
) {
	t.Helper()
	auth := filepath.Join(realUserHome(), ".codex", "auth.json")
	if _, err := os.Stat(auth); err != nil {
		t.Skip("no codex login on this machine")
	}
}

func suffixedKeychainService(
	configDir string,
) string {
	sum := sha256.Sum256([]byte(configDir))
	return "Claude Code-credentials-" + hex.EncodeToString(sum[:])[:8]
}

func keychainHasItem(
	t *testing.T,
	service string,
) bool {
	t.Helper()
	cmd := exec.Command("security", "find-generic-password", "-s", service)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func lookupCLI(
	t *testing.T,
	name string,
) string {
	t.Helper()
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	fallback := filepath.Join(realUserHome(), ".local", "bin", name)
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	t.Skipf("%s not installed", name)
	return ""
}

func runCLI(
	t *testing.T,
	bin string,
	args ...string,
) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v failed: %s", bin, args, out)
	return string(out)
}

func writeFile(
	t *testing.T,
	path string,
	body string,
) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}
