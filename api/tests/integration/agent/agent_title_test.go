//go:build integration

// This file closes the e2e gap for the DERIVED half of chat titling: neither
// agent_test.go nor agent_gaps_test.go ever drives a REAL `user_prompt` hook POST
// against the running daemon and reads a title back —
// internal/app/usecases/chat/rename_test.go's TestRenameChat_Precedence and
// TestIngestHook_UserPrompt_SetsDerivedTitle already prove RenameChat's precedence
// rules and deriveTitle's wiring at the usecase level, in-process, with a
// fakeCommander standing in for the real vendor CLI. What only this file proves is
// the OS-process boundary: that the actual compiled `crowbar` binary — the exact one
// a real vendor CLI's hook config shells out to on every prompt — reaches this
// package's real unix-socket daemon and the title lands, read back through
// Usecases.AgentChat.GetChat exactly as the API layer would.
//
// It used to cover an AGENT-driven half too, by exec'ing `crowbar chat rename`. That
// subcommand is gone: titling is the set_chat_title MCP tool now, and the shell path
// COMPETED with it — handed both, a model would read the tool list and then type the
// command instead, which is what made the tool's compliance unmeasurable.
// agent_mcp_test.go covers the tool against a real CLI.
package agent_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// mustSpawnChat creates a project + repo + managed child workspace and spawns
// a real <provider> chat in it (via the same importRepoAndWorkspace +
// SpawnChat sequence every other test in this package repeats), registering
// the spawned PTY's teardown so callers don't have to. Returns the workspace's
// project/repo/workspace ids (the in-PTY CLI callbacks now need all three to
// build the workspace-nested agent URL) along with the new chat's id and the id of
// the RUNNER started on it — which is the crowbarSegmentID every hook carries, and
// so is still what the `--segment` flag below is given.
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

// crowbarBinPath returns the real compiled crowbar binary this test's
// newHarness(t) call already built and wired as CROWBAR_HOOK_BIN (the same
// env seam agent.go's crowbarHookPath reads before falling back to
// <home>/bin/crowbar). This harness's from-scratch daemon (engine -> adapter
// -> app -> v0 router, built directly by newHarness rather than through
// internal.New) never runs internal.New's selfinstall.Install step, so there
// is no populated <home>/bin/crowbar in this package's tests to point at —
// CROWBAR_HOOK_BIN is the one real binary path this harness actually has, and
// it is the identical artifact production code already treats as "the
// installed crowbar" for shelling out. Must be called after newHarness(t).
func crowbarBinPath(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("CROWBAR_HOOK_BIN")
	require.NotEmpty(t, bin, "CROWBAR_HOOK_BIN unset — call newHarness(t) before crowbarBinPath(t)")
	return bin
}

// settleTitle blocks until the daemon has finished recording a title change, with
// no polling at all — because by this point there is nothing left to poll FOR.
//
// The tests in this file drive no PTY: they invoke the real `crowbar` binary as an
// OS subprocess, and its POST is handled SYNCHRONOUSLY by the gin handler (rename
// calls RenameChat, hook calls IngestHook, both before the response is written).
// So when CombinedOutput() returns, the subprocess has exited, which means the
// daemon already answered it, which means the aggregate is committed. The one thing
// that can still be in flight is the INDEPENDENT store projection the read-back goes
// through — and Quiesce is exactly the barrier for that: it returns when every
// projection has folded.
//
// The old helper re-read the chat every 100ms for 5 seconds. It was not waiting for
// the subprocess (already dead) but for that projection, and a poll cannot know when
// a projection is done — it can only guess that 5 seconds is enough.
func settleTitle(
	h *harness,
) {
	h.app.Repositories.WaitQuiescent()
}

// TestAgent_FirstPrompt_DerivesTitle proves the derived-title fallback end to
// end: a REAL `crowbar hook user_prompt` binary invocation — an OS subprocess
// standing in for the vendor CLI's own UserPromptSubmit hook shelling out per
// claude.yaml's config_injection ("{crowbar_hook} hook user_prompt --segment
// {segid} --provider {provider}") — POSTed against the running daemon derives
// and sets the chat's title from the prompt's first line (deriveTitle in
// agent.go), read back through Usecases.AgentChat.GetChat. The payload shape
// (`{"prompt": "..."}`) matches claude.yaml's hooks.events.user_prompt field
// map (message: prompt) exactly, and the prompt text is a single short line
// under deriveTitle's 60-rune truncation threshold, so the derived title must
// equal the prompt verbatim (internal/app/usecases/chat/rename_test.go's
// TestIngestHook_UserPrompt_SetsDerivedTitle already proves deriveTitle's
// first-line/60-rune behavior in-process; this test proves the same outcome
// reaching the daemon over the real HTTP+hook-CLI path instead of a direct
// IngestHook Go call).
func TestAgent_FirstPrompt_DerivesTitle(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	projectID, repoID, wsID, chatID, segID := mustSpawnChat(t, h, "claude")

	crowbar := crowbarBinPath(t)
	const prompt = "Explain how bloom filters work"
	out, err := exec.Command(crowbar, "hook", "user_prompt",
		"--segment", segID, "--provider", "claude",
		"--project", projectID, "--repo", repoID, "--workspace", wsID,
		"--payload", `{"prompt":"`+prompt+`"}`,
	).CombinedOutput()
	require.NoError(t, err, "exec crowbar hook user_prompt: %s", out)
	t.Logf("crowbar hook user_prompt output: %q", out)

	settleTitle(h)

	chat, err := h.app.Usecases.AgentChat.GetChat(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, prompt, chat.Title,
		"AgentChat.Title must equal the derived (first-line) title from the real user_prompt hook POST")
}
