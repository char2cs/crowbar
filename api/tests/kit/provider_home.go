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

// IsolateProviderHomes points the vendor CLIs at throwaway homes for the rest of
// the test, so the per-place records they keep land somewhere this test owns
// instead of in the user's REAL provider homes.
//
// This exists because a vendor CLI remembers trust OUTSIDE the temp home Crowbar
// controls, keyed by PLACE, and every test builds a fresh temp repo it will never
// see again: codex appends a [projects."<repo root>"] block to ~/.codex/config.toml
// and claude a key to ~/.claude.json. Nothing ever removes those, so the user's own
// config grew one dead stanza per test run, without bound.
//
// It is deliberately a TEST-ONLY seam. Crowbar itself must never own a provider's
// home — pointing CODEX_HOME at a directory Crowbar created and reaped once made it
// the custodian of codex's SESSIONS and destroyed them on a provider switch, which
// TestSwitchProvider_CodexKeepsItsOwnHome still pins. The rule being protected there
// is "the app does not relocate a provider's home", and a test process relocating its
// OWN children's homes does not weaken it. The env vars are set with t.Setenv on the
// test process, and the CLI inherits them through the daemon's os.Environ(); no
// descriptor and no usecase learns they exist.
//
// BOTH HALVES OF THE TRUST BARRIER SURVIVE, and they were verified live rather than
// assumed (codex-cli 0.146.0, claude 2.1.234): the homes start empty, so the FIRST
// CLI of a provider still paints its trust dialog for firstOfProvider to block on,
// and answering it records trust in the temp home, so every LATER CLI of that
// provider in the same repo still paints nothing.
func IsolateProviderHomes(
	t *testing.T,
) {
	t.Helper()
	root := tempHome(t)
	t.Setenv("CODEX_HOME", isolatedCodexHome(t, root))
	t.Setenv("CLAUDE_CONFIG_DIR", isolatedClaudeHome(t, root))
	// claude derives its KEYCHAIN SERVICE NAME from the config dir — the item is
	// "Claude Code-credentials" by default but "…-<sha256(CLAUDE_CONFIG_DIR)[:8]>"
	// the moment CLAUDE_CONFIG_DIR is set, which is the whole reason relocating the
	// config dir has always read as "CLAUDE_CONFIG_DIR breaks its auth": the
	// credentials are still in the keychain, under a name claude no longer looks up.
	//
	// CLAUDE_SECURESTORAGE_CONFIG_DIR is consulted FIRST for that name and an EMPTY
	// value selects the unsuffixed one, so this is the seam that lets the config dir
	// move while the credential lookup stays put. Empty is load-bearing and is not
	// the same as unset — Go passes it through os.Environ() as "KEY=", which the CLI
	// reads as a defined empty string.
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

// linkCodexAuth SYMLINKS the real auth.json rather than copying it: codex keeps its
// credentials in a file of their own inside its home, and a link authenticates the
// isolated home without this harness ever holding a copy of a token. Everything else
// codex wants (config, rollouts, caches, sqlite) it creates in the temp home itself.
//
// A missing file is not an error. A machine with no codex login has nothing to link,
// and the tests that need a live codex skip on their own.
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

// claudeSeed is the real ~/.claude.json with its "projects" map — and ONLY that map —
// emptied.
//
// The projects map is the pollution, so it is what gets dropped. Everything else is
// carried because those top-level keys are where claude records that its ONE-TIME
// modals have been dismissed, and a modal this harness does not expect is a hang, not
// a failure. A config seeded with nothing but hasCompletedOnboarding was tried first
// and is not enough: claude then paints its "Claude in Chrome extension detected"
// prompt AFTER the trust dialog, under the SAME "Enter to confirm" text the trust
// barrier keys on, so the harness answers the trust dialog and then blocks forever on
// a second modal it never knew existed.
//
// No credential is carried. claude's tokens live in the macOS keychain, which this
// seed does not touch and IsolateProviderHomes deliberately keeps pointing at the
// real item; oauthAccount and friends are account metadata, not secrets.
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

// realUserHome is the login account's home, read from the USER DATABASE and only
// falling back to $HOME, so it still resolves to the user's real provider homes from
// inside a test that has repointed HOME — and so the isolation and the guard that
// polices it can never disagree about which directory they are protecting.
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
