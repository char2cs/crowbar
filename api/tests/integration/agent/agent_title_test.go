//go:build integration

// This file holds the spawn helper the MCP titling tests share. Titling is the
// set_chat_title MCP tool (agent_mcp_test.go); a title derived from the first
// prompt is pinned in-process (TestIngestHook_UserPrompt_SetsDerivedTitle).
package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// mustSpawnChat creates a project + repo + managed child workspace and spawns
// a real <provider> chat in it (via the same importRepoAndWorkspace +
// SpawnChat sequence every other test in this package repeats), registering
// the spawned PTY's teardown so callers don't have to. Returns the workspace's
// project/repo/workspace ids along with the new chat's id and the id of
// the RUNNER started on it, which every hook is posted as.
func mustSpawnChat(
	t *testing.T,
	h *harness,
	provider string,
) (projectID, repoID, wsID, chatID, runnerID string) {
	t.Helper()
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	projectID, repoID, wsID = h.importRepoAndWorkspace(t, "title-"+provider, repoPath)

	chatID, runnerID, err := h.app.Usecases.AgentRunner.SpawnChat(ctx, wsID, provider)
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned %s: chat=%s runner=%s workspace=%s home=%s", provider, chatID, runnerID, wsID, h.home)

	termSessID := liveRunnerTerminalSession(t, h, chatID)
	require.NotEmpty(t, termSessID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	return projectID, repoID, wsID, chatID, runnerID
}
