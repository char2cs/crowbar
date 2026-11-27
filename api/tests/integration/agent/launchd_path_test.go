//go:build integration

package agent_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

const launchdMinimalPath = "/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin"

func TestRegression_SpawnChatUnderLaunchdMinimalPath(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			requireCLI(t, provider)

			h := newHarness(t)
			repoPath := kit.InitRepo(t)
			_, _, wsID := h.importRepoAndWorkspace(t, "launchd-path", repoPath)

			t.Setenv("PATH", launchdMinimalPath)
			require.NotContains(t, os.Getenv("PATH"), ".local/bin",
				"the repro requires a PATH that does NOT contain the CLI's install dir")

			chatID, runnerID, err := spawnChat(context.Background(), h, wsID, provider)
			require.NoError(t, err,
				"the vendor CLI must still spawn under launchd's minimal PATH; a bare argv[0] fails here with "+
					`"executable file not found in $PATH" — the error that surfaced as POST /chats 500`)
			require.NotEmpty(t, chatID)
			require.NotEmpty(t, runnerID)

			termSessID := liveRunnerTerminalSession(t, h, chatID)
			require.NotEmpty(t, termSessID)
			t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })
		})
	}
}
