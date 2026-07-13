//go:build integration

// This file closes Task 8's e2e gap for the chat-title feature: neither
// agent_test.go nor agent_gaps_test.go ever drives the REAL `crowbar chat
// rename` binary or a REAL `user_prompt` hook POST against the running daemon
// and reads a title back — internal/app/usecases/agent/rename_test.go's
// TestRenameChat_Precedence and TestIngestHook_UserPrompt_SetsDerivedTitle
// already prove RenameChat's precedence rules and deriveTitle's wiring at the
// usecase level, in-process, with a fakeCommander standing in for the real
// vendor CLI. What has never been proven is the OS-process boundary: that the
// actual compiled `crowbar` binary — the exact one a real agent would exec
// following its injected title_instruction, and the exact one a real vendor
// CLI's hook config shells out to on every prompt — reaches this package's
// real unix-socket daemon and the title lands, read back through
// Usecases.Agent.GetChat exactly as the API layer would.
package agent_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

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

	chatID, runnerID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, provider)
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

// waitUntil polls check every 100ms until it returns true or timeout elapses,
// failing the test if it never does. Unlike agent_test.go's nudgeUntil, the
// tests in this file never drive a PTY: both the rename binary's POST and the
// hook binary's POST are handled synchronously by their gin handlers (Rename
// calls RenameChat, Hooks calls IngestHook, both before writing the response)
// before the subprocess ever returns, so there is nothing to nudge past — a
// bare poll loop is enough, kept bounded only as a defensive margin against
// any incidental async broadcast work.
func waitUntil(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			require.Fail(t, "waitUntil: condition never became true", "timeout=%s", timeout)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestAgent_CrowbarChatRename_SetsTitle proves the agent-facing command path:
// invoking the REAL `crowbar chat rename` binary — an OS subprocess, not an
// in-process usecase call — against the running daemon updates the
// AgentChat's title with agent precedence (source=agent), read back through
// Usecases.Agent.GetChat exactly as the API/FE layer would. This is the exact
// binary + argv (`crowbar chat rename <chatid> "<title>"`) a real agent would
// run after reading its injected title_instruction
// (TestSpawnChat_InjectsTitleInstruction in
// internal/app/usecases/agent/rename_test.go already proves that instruction
// text names this exact subcommand), and the exact command cmd/crowbar/chat.go's
// runChatRename posts to POST /v0/agent/chats/:id/rename?source=agent.
//
// Note runChatRename's RunE always exits 0, swallowing any daemon-reported
// error to stderr only (cmd/crowbar/chat.go: "Run by the agent — must never
// break its turn"), and ipc.Client.PostJSON itself never surfaces a non-2xx
// status as a Go error either — so a clean subprocess exit here is NOT proof
// the rename actually landed. The real assertion is the bounded read-back
// below through GetChat.
func TestAgent_CrowbarChatRename_SetsTitle(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	projectID, repoID, wsID, chatID, runnerID := mustSpawnChat(t, h, "claude")

	crowbar := crowbarBinPath(t)
	const wantTitle = "Fix The Auth Flow"
	// `chat rename --segment <segid> <title>` — the argv the real binary now takes, and
	// the chat id is deliberately NOT in it. The agent is never told which chat it is on:
	// a /clear or /resume moves its CLI, and an id baked into its spawn-time instruction
	// would then name a chat it has walked out of. It names its RUNNER, and the daemon
	// resolves runner → its CURRENT chat at call time (cmd/crowbar/chat.go's ExactArgs(1);
	// POST .../agent/runners/:segid/rename).
	out, err := exec.Command(crowbar, "chat", "rename",
		"--segment", runnerID,
		"--project", projectID, "--repo", repoID, "--workspace", wsID,
		wantTitle).CombinedOutput()
	require.NoError(t, err, "exec crowbar chat rename: %s", out)
	t.Logf("crowbar chat rename output: %q", out)

	waitUntil(t, 5*time.Second, func() bool {
		c, err := h.app.Usecases.Agent.GetChat(ctx, chatID)
		require.NoError(t, err)
		return c.Title == wantTitle
	})

	chat, err := h.app.Usecases.Agent.GetChat(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, wantTitle, chat.Title,
		"AgentChat.Title must equal the title the real crowbar chat rename binary posted")
}

// TestAgent_FirstPrompt_DerivesTitle proves the derived-title fallback end to
// end: a REAL `crowbar hook user_prompt` binary invocation — an OS subprocess
// standing in for the vendor CLI's own UserPromptSubmit hook shelling out per
// claude.yaml's config_injection ("{crowbar_hook} hook user_prompt --segment
// {segid} --provider {provider}") — POSTed against the running daemon derives
// and sets the chat's title from the prompt's first line (deriveTitle in
// agent.go), read back through Usecases.Agent.GetChat. The payload shape
// (`{"prompt": "..."}`) matches claude.yaml's hooks.events.user_prompt field
// map (message: prompt) exactly, and the prompt text is a single short line
// under deriveTitle's 60-rune truncation threshold, so the derived title must
// equal the prompt verbatim (internal/app/usecases/agent/rename_test.go's
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

	waitUntil(t, 5*time.Second, func() bool {
		c, err := h.app.Usecases.Agent.GetChat(ctx, chatID)
		require.NoError(t, err)
		return c.Title == prompt
	})

	chat, err := h.app.Usecases.Agent.GetChat(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, prompt, chat.Title,
		"AgentChat.Title must equal the derived (first-line) title from the real user_prompt hook POST")
}
