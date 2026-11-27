//go:build integration

package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func mustSpawnChat(
	t *testing.T,
	h *harness,
	provider string,
) (projectID, repoID, wsID, chatID, runnerID string) {
	t.Helper()
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	projectID, repoID, wsID = h.importRepoAndWorkspace(t, "title-"+provider, repoPath)

	chatID, runnerID, err := spawnChat(ctx, h, wsID, provider)
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned %s: chat=%s runner=%s workspace=%s home=%s", provider, chatID, runnerID, wsID, h.home)

	termSessID := liveRunnerTerminalSession(t, h, chatID)
	require.NotEmpty(t, termSessID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	return projectID, repoID, wsID, chatID, runnerID
}
