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

// TestIsolateProviderHomes_CodexRunsAuthenticatedOutOfTheTempHome is the live proof
// that the codex half of the isolation works, and it drives a REAL codex to get it.
//
// Two things have to be true at once, and only one of them is obvious. The obvious one
// is that CODEX_HOME moves codex's home. The other is that codex is still LOGGED IN
// there — a relocated home is worthless if every test then fails on authentication,
// and a fresh home has no credentials of its own. IsolateProviderHomes links codex's
// auth.json in rather than copying it, and this asserts that link is enough.
//
// `codex login status` is the probe because it reads the credential and nothing else:
// no model turn, so it costs no quota and cannot fail for a rate limit. (`codex debug
// models` was tried first and is useless here — it answers identically with no
// credentials at all.)
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

// TestIsolateProviderHomes_ClaudeKeychainLookupStaysOnTheRealCredential pins the one
// subtlety that makes claude's isolation possible at all, and it pins it by checking
// the thing that actually breaks.
//
// claude names its keychain item after its config dir: "Claude Code-credentials" with
// no CLAUDE_CONFIG_DIR, and "Claude Code-credentials-<sha256(dir)[:8]>" with one. That
// suffix is the entire reason "CLAUDE_CONFIG_DIR breaks claude's auth" has been true
// every time anyone tried it — the credential is still in the keychain, under a name
// claude has stopped asking for. CLAUDE_SECURESTORAGE_CONFIG_DIR is consulted first
// for that name and an EMPTY value selects the unsuffixed one.
//
// So this asserts both sides: the name claude WILL look up resolves to a real item,
// and the name it would have looked up WITHOUT the override resolves to nothing. The
// second assertion is the one with teeth — it is the failure this override exists to
// prevent, demonstrated rather than described.
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

// TestIsolateProviderHomes_ClaudeConfigIsSeededWithNoPlaces pins the shape of the
// seeded config: it must carry the real config's one-time-modal flags (or the CLI
// paints a dialog the harness has no barrier for and hangs), and it must carry NO
// projects (or the isolation has imported the very litter it exists to stop).
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

// TestProviderHomeRecord_AddedNamesEveryNewPlace is the guard's own test: it feeds the
// scrapers a before/after pair and requires the added places — and only those — to come
// back named.
//
// It runs against files rather than the real homes for the obvious reason: the only way
// to test this end to end would be to pollute the very file the whole change exists to
// protect. The CONTENT is not invented though — codexTrustStanzas is the shape a real
// codex-cli 0.146.0 wrote when it was pointed at a throwaway home and answered its own
// trust dialog, blank line between stanzas and all, because a scraper tuned to a
// plausible-looking fixture is a scraper that has never met the file it must read.
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

// TestSnapshotProviderHomes_SurvivesAbsentHomes keeps the guard from becoming the
// thing that breaks CI: a machine with no codex and no claude installed must snapshot
// clean and compare clean rather than error.
func TestSnapshotProviderHomes_SurvivesAbsentHomes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	assert.Empty(t, codexProjectKeys(missing))
	assert.Empty(t, claudeProjectKeys(missing))

	snap := SnapshotProviderHomes()
	assert.Empty(t, snap.Added(snap))
}

// oneTimeModalFlags are the ~/.claude.json keys that record a one-time dialog as
// already answered. They are listed so the seed's job is legible; the seed itself
// copies every top-level key, so this is an assertion, not the mechanism.
var oneTimeModalFlags = []string{
	"hasCompletedOnboarding",
	"hasCompletedClaudeInChromeOnboarding",
	"theme",
}

// codexTrustStanzas renders the projects table exactly as codex-cli writes it: one
// bracketed stanza per place, a trust_level line, and a BLANK LINE between stanzas.
// That blank line is the reason this is a helper and not a literal — a scraper that
// only ever saw stanzas packed back to back would not be tested against the real file.
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

// suffixedKeychainService rebuilds the service name claude would look up if
// CLAUDE_CONFIG_DIR were set and the securestorage override were not.
func suffixedKeychainService(
	configDir string,
) string {
	sum := sha256.Sum256([]byte(configDir))
	return "Claude Code-credentials-" + hex.EncodeToString(sum[:])[:8]
}

// keychainHasItem reports whether a generic-password item exists under service. It
// never passes -w, so no secret is ever read, let alone printed.
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
