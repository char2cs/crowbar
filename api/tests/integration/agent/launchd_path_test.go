//go:build integration

package agent_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// launchdMinimalPath is the PATH a macOS .app daemon started by launchd actually
// inherits. It is not a stand-in for the production daemon's environment — it IS
// that environment, read off the running packaged daemon.
const launchdMinimalPath = "/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin"

// TestRegression_SpawnChatUnderLaunchdMinimalPath guards the bug where the packaged
// .app could not open ANY chat: every click POSTed /agent/chats and got a 500 back,
// so the chat button looked dead.
//
// The spawn exec'd a BARE argv[0] ("claude"), and Go resolves a bare name via
// exec.LookPath against the DAEMON's OWN PATH — cmd.Env is ignored for the lookup, so
// the spawn plan's carefully built env could not rescue it. Under launchd that PATH is
// the five dirs below, which do not include ~/.local/bin, where claude and codex
// install. core/shellenv repairs the PATH from a LOGIN NON-INTERACTIVE shell, which
// sources .zprofile/.zshenv but never .zshrc — the file that conventionally adds
// ~/.local/bin — so the repair did not reach it either. Every spawn died with
// "executable file not found in $PATH".
//
// Pinning the process PATH is the whole point: with a developer shell's PATH this test
// passes whether or not the bug is fixed. The fix is that argv[0] is resolved through
// binpath, which probes the well-known install dirs a login-shell PATH never surfaces.
func TestRegression_SpawnChatUnderLaunchdMinimalPath(t *testing.T) {
	// EVERY provider, not just claude: they all install into ~/.local/bin and they all
	// spawn through the same seam, so a fix proven on one proves nothing about the other.
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			requireCLI(t, provider)

			// The harness first, while PATH is still the developer's: it shells out to `go
			// build` for the real crowbar hook binary, which needs the toolchain on PATH.
			// Only the SPAWN is put under launchd's PATH, which is the layer under test.
			h := newHarness(t)
			repoPath := kit.InitRepo(t)
			_, _, wsID := h.importRepoAndWorkspace(t, "launchd-path", repoPath)

			t.Setenv("PATH", launchdMinimalPath)
			require.NotContains(t, os.Getenv("PATH"), ".local/bin",
				"the repro requires a PATH that does NOT contain the CLI's install dir")

			chatID, runnerID, err := h.app.Usecases.AgentRunner.SpawnChat(context.Background(), wsID, provider)
			require.NoError(t, err,
				"the vendor CLI must still spawn under launchd's minimal PATH; a bare argv[0] fails here with "+
					`"executable file not found in $PATH" — the error that surfaced as POST /agent/chats 500`)
			require.NotEmpty(t, chatID)
			require.NotEmpty(t, runnerID)

			// A live PTY exists, so the process genuinely came up — rather than SpawnChat
			// merely recording a runner for a CLI that never started.
			termSessID := liveRunnerTerminalSession(t, h, chatID)
			require.NotEmpty(t, termSessID)
			t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })
		})
	}
}
