package agent_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// ---------------------------------------------------------------------------
// The bugs. One test per bug, each named for what it locks out.
// ---------------------------------------------------------------------------

// TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource
//
// The user's bug, exactly as they hit it: runner R1 is on chat A. Inside its CLI the
// user /resume's into chat B's conversation — and B ALREADY has its own live runner
// R2. The old code ran EndSegment(A) (committed), then OpenSegment(B) — which FAILED,
// because OpenSegment.Validate rejects a chat that already has an active segment.
// There was no rollback: chat A was left with no active segment and permanently
// unusable, and B was never joined. Both CLIs kept running, one of them invisible.
//
// A move is now ONE write to ONE aggregate. The chat being left is not written to at
// all, so no failure anywhere can damage it.
func TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude") // R1 on A, conversation sA
	f.announce(t, r1, "sA")
	chatB, r2 := f.spawn(t, "codex") // R2 on B, conversation sB
	f.announce(t, r2, "sB")

	r2runner := f.runner(t, r2)

	// R1's CLI resumes into B's conversation. The CLI has ALREADY switched; Crowbar
	// can only record it.
	f.announce(t, r1, "sB")

	// Chat A must still be usable: dormant, but with its history intact and
	// resumable. It must NOT be a chat with no way back.
	last, err := f.runners.LastConversation(f.ctx, chatA)
	require.NoError(t, err, "chat A keeps its conversation history and stays resumable")
	assert.Equal(t, "sA", last.SessionID)
	assert.Equal(t, "claude", last.ProviderID)

	_, err = f.chats.GetChat(f.ctx, chatA)
	require.NoError(t, err, "chat A still exists")

	_, err = f.liveRunnerFor(t, chatA)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "chat A is dormant — nothing points at it — not broken")

	// R1 now holds chat B.
	live, err := f.liveRunnerFor(t, chatB)
	require.NoError(t, err)
	assert.Equal(t, r1, live.ID, "the mover took the chat over")
	assert.Equal(t, "sB", live.CurrentSession)

	// R2 was evicted — TerminateGraceful'd (invariant I3: at most one live runner per
	// conversation, or the provider's own session file gets two writers).
	assert.Contains(t, f.term.terminatedIDs(), r2runner.TerminalSession,
		"the incumbent holding the conversation was terminated")
}

// TestRegression_ResumeIntoOccupiedChat_SourceStaysResumable is the other half of the
// same bug: a chat that was bricked could never be re-entered by ANY route. Prove the
// way back works — the source chat resumes, into its own conversation.
func TestRegression_ResumeIntoOccupiedChat_SourceStaysResumable(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	// A real turn, so sA is a conversation that exists on disk and can be resumed.
	turn(t, f, r1, "claude", "claude said something in A")

	_, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "sB")

	f.announce(t, r1, "sB") // R1 leaves A for B

	revived, err := f.usecase.ResumeChat(f.ctx, chatA)
	require.NoError(t, err, "the vacated chat must be revivable")
	require.NotEmpty(t, revived)
	f.wait()

	live, err := f.liveRunnerFor(t, chatA)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID, "chat A is live again, on a new runner")
	assert.Equal(t, "claude", live.ProviderID, "revived by the provider that was last there")

	// And it resumed the CLI's own conversation rather than starting a blank one.
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "sA", argAfter(t, argv, "--resume"))
}

// TestRegression_ClearMintsChat_KeepsSamePTY: /clear (an unknown conversation id
// appears under a live runner) mints a chat and moves the runner into it — WITHOUT
// touching the PTY. Same process, same terminal, same runner id: that is what lets the
// open pane relabel instead of remounting its terminal.
func TestRegression_ClearMintsChat_KeepsSamePTY(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "s1")
	before := f.runner(t, r1)

	require.Equal(t, 1, f.term.callCount())

	f.announce(t, r1, "s2") // /clear: a conversation nobody has seen

	after := f.runner(t, r1)
	assert.NotEqual(t, chatA, after.CurrentChatID, "the runner moved into a freshly minted chat")
	assert.Equal(t, "s2", after.CurrentSession)
	assert.Equal(t, before.TerminalSession, after.TerminalSession, "the PTY must not change on a /clear")
	assert.Equal(t, before.ID, after.ID, "the runner id is stable across the move")
	assert.Equal(t, 1, f.term.callCount(), "no new CLI may be spawned for a /clear")
	assert.Empty(t, f.term.terminateRequestIDs(), "and nothing may be terminated")

	// The new chat exists, in the same workspace, and is the one the runner is on.
	newChat := f.chat(t, after.CurrentChatID)
	assert.Equal(t, "ws1", newChat.WorkspaceID)
	live, err := f.liveRunnerFor(t, newChat.ID)
	require.NoError(t, err)
	assert.Equal(t, r1, live.ID)

	// The vacated chat is untouched and dormant — never written to.
	_, err = f.liveRunnerFor(t, chatA)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestRegression_TurnHookAfterMove_LandsInTheChatTheRunnerIsOnNow is bug 2, the split
// brain. The old reducer mutated an in-memory segment→chat map BEFORE the aggregate
// commands ran, so a failed command left the registry believing a move had happened
// that had not — and the orphaned CLI's turn hooks were routed into a chat it had
// left. Routing now resolves runner → CurrentChatID from DURABLE state, so there is
// nothing left to disagree with reality.
func TestRegression_TurnHookAfterMove_LandsInTheChatTheRunnerIsOnNow(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "s1")
	turn(t, f, r1, "claude", "said in the first chat")

	f.announce(t, r1, "s2") // /clear
	chatB := f.runner(t, r1).CurrentChatID
	turn(t, f, r1, "claude", "said after the move")

	handoffB, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.Contains(t, handoffB, "said after the move", "the turn lands in the chat the runner is on NOW")

	handoffA, err := f.usecase.AssembleHandoff(f.ctx, chatA)
	require.NoError(t, err)
	assert.Contains(t, handoffA, "said in the first chat")
	assert.NotContains(t, handoffA, "said after the move",
		"a turn must never be filed into a chat the runner has left")
}

// TestRegression_MoveIntoKnownChat_IsRecordedEvenIfTheEvictionKillFails: reconcile,
// never transact. The CLI has already joined the conversation, so the record of that
// must not depend on our being able to kill anybody. If the eviction fails, the record
// is still ACCURATE — two runners really do hold the conversation — and only the
// cleanup is owed.
func TestRegression_MoveIntoKnownChat_IsRecordedEvenIfTheEvictionKillFails(t *testing.T) {
	f := newFixture(t)

	_, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	chatB, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "sB")

	f.term.terminateErr = errors.New("boom: the incumbent refuses to die")

	f.announce(t, r1, "sB")

	live, err := f.liveRunnerFor(t, chatB)
	require.NoError(t, err)
	assert.Equal(t, r1, live.ID, "the move is recorded even though the eviction failed")
	assert.Contains(t, f.term.terminateRequestIDs(), f.runner(t, r2).TerminalSession,
		"and the eviction was still attempted")
}

// ---------------------------------------------------------------------------
// Spawn
// ---------------------------------------------------------------------------

func TestSpawnChat_CreatesChatAndRunner_AndSpawnsTheCLI(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)

	chat := f.chat(t, chatID)
	assert.Equal(t, "ws1", chat.WorkspaceID)

	r := f.runner(t, runnerID)
	assert.Equal(t, "claude", r.ProviderID)
	assert.Equal(t, chatID, r.CurrentChatID)
	assert.Equal(t, "ws1", r.WorkspaceID)
	assert.NotEmpty(t, r.TerminalSession)
	assert.Empty(t, r.CurrentSession, "no conversation is bound until the provider announces one")

	// The chat is live because a runner points at it — not because of a flag on it.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	assert.Equal(t, "ws1", call.workspaceID)
	assert.Equal(t, f.ws.worktree, call.cwd)
	assert.Equal(t, "claude", filepath.Base(call.argv[0]))
	// A fresh SpawnChat injects the capability preamble via the descriptor's
	// context_inject mechanism (claude.yaml maps it to --append-system-prompt); it
	// must be present, not the raw ledger handoff (there is none yet for a
	// brand-new chat).
	assert.Contains(t, call.argv, "--append-system-prompt")
}

// TestSpawnRunner_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenThePTYDies guards the
// resource-leak fix: the per-spawn tmp dir (the rendered hook config the CLI is pointed
// at) must still exist while that CLI runs, and be removed when its PTY dies.
func TestSpawnRunner_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenThePTYDies(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	// <chatsDir>/runners/<runnerID>-<provider>: keyed by the RUNNER, not the chat, so the
	// dir stays findable from a bare runner row even after Displace erases its chat pointer
	// (see worktreepath.RunnerDir).
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	info, err := os.Stat(tmpDir)
	require.NoError(t, err, "the tmp dir must exist immediately after spawn")
	assert.True(t, info.IsDir())

	require.Equal(t, 1, f.term.callCount())
	require.NotNil(t, f.term.calls[0].onExit, "CreateCommand must receive a non-nil onExit")

	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "and must survive while the CLI is running")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)

	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "the tmp dir must be removed once the PTY dies")
}

func TestSpawnChat_UsesDescriptorCmdAsArgv0(t *testing.T) {
	for _, providerID := range []string{"claude", "codex"} {
		t.Run(providerID, func(t *testing.T) {
			f := newFixture(t)

			_, runnerID := f.spawn(t, providerID)
			require.NotEmpty(t, runnerID)

			require.Equal(t, 1, f.term.callCount())
			// Basename, not the whole token: argv[0] is run through binpath.Resolve,
			// so it is the CLI's absolute path wherever the binary is installed and
			// the bare name only when it is nowhere to be found.
			assert.Equal(t, providerID, filepath.Base(f.term.calls[0].argv[0]))
		})
	}
}

// TestSpawnChat_ResolvesArgv0ToAbsolutePath pins the fix for the packaged .app
// failing to open ANY chat. exec.Command resolves a bare argv[0] against the
// DAEMON's PATH (cmd.Env is ignored for lookup), and a launchd-started daemon
// has a minimal PATH that misses ~/.local/bin — where claude and codex install.
// Every spawn therefore died with "executable file not found in $PATH", which
// surfaced as POST /agent/chats 500 and a chat button that did nothing.
//
// argv[0] must come out of binpath.Resolve so the CLI is exec'd by absolute path.
func TestSpawnChat_ResolvesArgv0ToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	// The CLI is reachable ONLY through this dir, exactly as it is reachable only
	// through ~/.local/bin in the failing install.
	t.Setenv("PATH", dir)

	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	require.NotEmpty(t, runnerID)

	require.Equal(t, 1, f.term.callCount())
	assert.Equal(t, bin, f.term.calls[0].argv[0])
}

func TestSpawnRunner_CrowbarHookPathFallsBackToHomeBinCrowbar(t *testing.T) {
	f := newFixture(t)
	// Override the fixture's default CROWBAR_HOOK_BIN so the path falls back to
	// <home>/bin/crowbar.
	t.Setenv("CROWBAR_HOOK_BIN", "")
	home := f.ws.home

	f.spawn(t, "claude")

	require.Equal(t, 1, f.term.callCount())
	settingsPath := argAfter(t, f.term.calls[0].argv, "--settings")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), filepath.Join(home, "bin", "crowbar")+" hook")
}

// TestSpawn_HookConfigCarriesRunnerAndProvider guards the arg-based spawn attribution:
// the RUNNER id (the crowbarSegmentID every hook carries) and the provider are
// rendered into the hook config command line, not injected as an env var.
func TestSpawn_HookConfigCarriesRunnerAndProvider(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	settingsPath := argAfter(t, call.argv, "--settings")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "--segment "+runnerID+" --provider claude")

	for _, kv := range call.env {
		assert.False(t, strings.HasPrefix(kv, "CROWBAR_SEGMENT_ID="), "env must not carry CROWBAR_SEGMENT_ID: %q", kv)
	}
}

// ---------------------------------------------------------------------------
// session_start — the context-move reducer's four outcomes
// ---------------------------------------------------------------------------

func TestIngestHook_SessionStart_Bind_RecordsTheConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()
	f.rbc.reset()

	f.announce(t, runnerID, "sid-abc")

	r := f.runner(t, runnerID)
	assert.Equal(t, "sid-abc", r.CurrentSession)
	assert.Equal(t, chatID, r.CurrentChatID, "a bind stays put: the runner does not move")
	assert.False(t, r.CurrentSessionSince.IsZero(), "the conversation is stamped with when it opened")

	// And the conversation is now KNOWN: history resolves it back to this chat, which
	// is what makes a later /resume into it recognisable rather than new.
	assert.Equal(t, chatID, f.chatForSession(t, "sid-abc"))

	assert.Equal(t, []string{"session_bound"}, f.runnerKinds(t))
	assert.Empty(t, f.bcKinds(t), "binding a conversation writes nothing to the chat")
}

// TestIngestHook_SessionStart_FirstAnnouncementOfAKnownID_BindsInPlace: the trap case.
// The FIRST id a runner announces must BIND, never MOVE — even when that id is already
// known, which is exactly what a resumed spawn (ResumeChat / a switch-back) looks like:
// the CLI comes up already inside the conversation we told it to resume.
func TestIngestHook_SessionStart_FirstAnnouncementOfAKnownID_BindsInPlace(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	turn(t, f, r1, "claude", "content so sA is resumable")
	f.term.exit(t, f.runner(t, r1).TerminalSession) // the CLI exits; chat A goes dormant
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatA)
	require.NoError(t, err)
	f.wait()

	// The revived CLI comes up inside sA and announces it. sA is KNOWN (chat A owns
	// it) — but this is the runner's first announcement, so it must bind in place.
	f.announce(t, revived, "sA")

	r := f.runner(t, revived)
	assert.Equal(t, chatA, r.CurrentChatID, "the resumed runner stays on the chat it was spawned into")
	assert.Equal(t, "sA", r.CurrentSession)
	assert.Empty(t, f.term.terminateRequestIDs(), "and nothing is evicted: the runner did not move")
}

func TestIngestHook_SessionStart_SameConversationIsANoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.rbc.reset()

	f.announce(t, runnerID, "sid-1")
	f.announce(t, runnerID, "sid-1")

	assert.Equal(t, []string{"session_bound"}, f.runnerKinds(t),
		"re-announcing the same conversation issues no command at all")
	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
}

func TestIngestHook_SessionStart_MoveToKnown_DormantChat_NoEviction(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	f.announce(t, r1, "sB") // /clear into a new chat
	chatB := f.runner(t, r1).CurrentChatID

	// /resume back into sA. Chat A is dormant (nothing points at it), so there is no
	// incumbent to evict — the runner simply returns.
	f.announce(t, r1, "sA")

	r := f.runner(t, r1)
	assert.Equal(t, chatA, r.CurrentChatID)
	assert.Equal(t, "sA", r.CurrentSession)
	assert.Empty(t, f.term.terminateRequestIDs(), "a dormant target has no incumbent to evict")

	_, err := f.liveRunnerFor(t, chatB)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "the chat it left is now dormant")
}

// TestIngestHook_SessionStart_MoveToKnown_SameChat_DoesNotEvictItself: a runner that
// announces a different conversation belonging to the chat it is ALREADY on must not
// evict itself.
func TestIngestHook_SessionStart_MoveToKnown_SameChat_DoesNotEvictItself(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	f.announce(t, r1, "sB") // moved to a new chat B
	chatB := f.runner(t, r1).CurrentChatID
	require.NotEqual(t, chatA, chatB)

	// Announce sB again after a no-op announce of sB — the runner is on B holding sB.
	f.announce(t, r1, "sB")

	assert.Empty(t, f.term.terminateRequestIDs(), "a runner must never evict itself")
	assert.Equal(t, chatB, f.runner(t, r1).CurrentChatID)
}

func TestIngestHook_SessionStart_EmptySessionID_IsIgnored(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.rbc.reset()

	f.announce(t, runnerID, "")

	assert.Empty(t, f.runnerKinds(t), "an announcement with no conversation id is not an event")
	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
}

// ---------------------------------------------------------------------------
// Turn hooks
// ---------------------------------------------------------------------------

func TestIngestHook_TurnStopAppendsAssistantTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"})))

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "assistant (claude): done thing")

	// turn_stop closes the turn (StopTurn → agentchat.turn_stopped); the ledger append
	// itself emits no aggregate event, so exactly one frame lands.
	assert.Equal(t, []string{"turn_stopped"}, f.bcKinds(t))
}

// TestIngestHook_UserPromptAppendsUserTurn: user_prompt appends a user turn, fires the
// derived-title fallback first (empty title), and opens the turn.
func TestIngestHook_UserPromptAppendsUserTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "please do the thing"})))

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "user: please do the thing")

	// The derived-title SetTitle (title_set) then StartTurn (turn_started) each emit
	// exactly one frame, in that order; the ledger append emits none.
	assert.Equal(t, []string{"title_set", "turn_started"}, f.bcKinds(t))
}

// TestIngestHook_TurnStop_EmptyMessage_AppendsNoLedgerTurnButStillClosesTurn: an empty
// message is a ledger no-op, not a turn-state no-op.
func TestIngestHook_TurnStop_EmptyMessage_AppendsNoLedgerTurnButStillClosesTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "sid-1"})))

	assert.Equal(t, []string{"turn_stopped"}, f.bcKinds(t))

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, handoff, "an empty assistant message must append no ledger turn")
}

// TestIngestHook_UserPromptOpensTurn_TurnStopClosesTurn: the hooks drive the chat's
// live turn state, which is what the workspace's working spinner reads.
func TestIngestHook_UserPromptOpensTurn_TurnStopClosesTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.False(t, f.chat(t, chatID).Working, "a fresh chat is not Working")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "start working"})))
	working := f.chat(t, chatID)
	assert.True(t, working.Working, "a user_prompt must open the turn (Working==true)")
	require.NotNil(t, working.CurrentTurnStarted, "an open turn must record CurrentTurnStarted")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "done"})))
	idle := f.chat(t, chatID)
	assert.False(t, idle.Working, "a turn_stop must close the turn (Working==false)")
	assert.Nil(t, idle.CurrentTurnStarted, "a closed turn must clear CurrentTurnStarted")
}

func TestIngestHook_UnknownRunner_IsIgnored(t *testing.T) {
	f := newFixture(t)

	require.NoError(t, f.usecase.IngestHook(f.ctx, "does-not-exist", "claude", "session_start",
		mustJSON(t, map[string]any{"session_id": "sid-1"})))
	assert.Empty(t, f.bc.snapshot())
	assert.Empty(t, f.rbc.snapshot(), "a hook must never resurrect a runner we do not know")
}

// TestIngestHook_ExitedRunner_IsIgnored: once the PTY is gone the runner is gone, and a
// late hook from it changes nothing. (Liveness has exactly one source: the process.)
func TestIngestHook_ExitedRunner_IsIgnored(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	f.bc.reset()
	f.rbc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "a ghost speaks"})))

	assert.Empty(t, f.bcKinds(t))
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, handoff, "a dead runner's hook must not write to the ledger")
}

func TestIngestHook_UnmappedCanonicalEvent_ReturnsNil(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "not_a_real_hook", mustJSON(t, map[string]any{})))
	assert.Empty(t, f.bcKinds(t), "an unmapped hook issues no command and so emits no frame")
}

// ---------------------------------------------------------------------------
// Runner exit
// ---------------------------------------------------------------------------

// TestOnExit_DeadPTY_MeansDeadRunner: the PTY is the sole authority on liveness. When
// it dies the runner is gone from the live model — and the chat is dormant, not broken.
func TestOnExit_DeadPTY_MeansDeadRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "no runner may outlive its PTY")
	_, err = f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "the chat is dormant")

	// ...but its conversation history survives the process, so it stays resumable.
	last, err := f.runners.LastConversation(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "s1", last.SessionID)
}

// TestOnExit_MidTurn_ClosesTheTurn: a CLI that dies mid-turn never sends its turn_stop,
// so without this the chat would spin forever.
func TestOnExit_MidTurn_ClosesTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "do work"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the turn is open")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)

	assert.False(t, f.chat(t, chatID).Working, "a dead CLI cannot still be mid-turn")
}

// TestOnExit_AfterSwitch_DoesNotCloseTheIncomingRunnersTurn: a provider switch starts
// the incoming CLI while the outgoing one is still dying (SIGTERM is not synchronous),
// so the outgoing runner's belated exit must not close the INCOMING runner's turn.
func TestOnExit_AfterSwitch_DoesNotCloseTheIncomingRunnersTurn(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	oldTerm := f.runner(t, oldRunner).TerminalSession

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(f.ctx, newRunner, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the new CLI is working"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the INCOMING runner has an open turn")

	// The outgoing CLI finally dies.
	f.term.exit(t, oldTerm)
	f.wait()

	assert.True(t, f.chat(t, chatID).Working,
		"the outgoing runner's exit must not close the incoming runner's turn")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID, "and the incoming runner still holds the chat")
}

// ---------------------------------------------------------------------------
// Broadcast
// ---------------------------------------------------------------------------

// TestBroadcast_ChatAndRunnerFramesAreDistinct pins the single-broadcaster invariant
// across both aggregates: each event produces exactly one frame on its own feed, in
// order, and a runner move produces NO chat event at all — because the chat is not
// written to.
func TestBroadcast_ChatAndRunnerFramesAreDistinct(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hello"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "hi"})))
	f.wait()
	f.announce(t, runnerID, "s2") // /clear: mints a chat, moves the runner
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Equal(t, []string{
		"created",      // SpawnChat
		"title_set",    // user_prompt: derived title
		"turn_started", // user_prompt: StartTurn
		"turn_stopped", // turn_stop: StopTurn
		"created",      // /clear: the minted chat
		"deleted",      // PurgeChat (asynx Forget's OnForget)
	}, f.bcKinds(t))

	assert.Equal(t, []string{
		"started",       // SpawnChat
		"session_bound", // the first conversation announcement
		"moved",         // /clear
	}, f.runnerKinds(t))
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestSpawnChat_CreateChatFailure_TearsDownCLIAndWraps(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	cs.failCreate = fmt.Errorf("boom: create")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create chat")

	// The CLI was already spawned (term-1) and must be torn down so no orphan process
	// leaks when the persist fails.
	require.Equal(t, 1, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-1")
}

// TestSpawnChat_StartRunnerFailure_TearsDownCLIAndWraps: a CLI whose runner cannot be
// recorded is invisible to Crowbar. Kill it rather than leak it.
func TestSpawnChat_StartRunnerFailure_TearsDownCLIAndWraps(t *testing.T) {
	f, _, rs := newFaultFixture(t)
	rs.failStart = fmt.Errorf("boom: start runner: %w", asynxModels.ErrValidation)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start runner")
	assert.ErrorIs(t, err, asynxModels.ErrValidation, "a conflict must still classify as one upstream")

	require.Equal(t, 1, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-1",
		"a CLI nothing points at must never be left running")
}

func TestSpawnChat_CreateCommandFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.term.err = fmt.Errorf("boom: create command")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create command")
}

func TestSpawnChat_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.ws.err = fmt.Errorf("boom: worktree lookup")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestSpawnChat_UnknownProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
}

func TestIngestHook_RunnerLookupFailure_ReturnsWrappedError(t *testing.T) {
	f, _, rs := newFaultFixture(t)

	_, runnerID := f.spawn(t, "claude")

	rs.failMove = fmt.Errorf("boom: move")
	f.announce(t, runnerID, "s1")

	// A move that fails surfaces, wrapped — and, crucially, has destroyed nothing.
	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", "session_start",
		mustJSON(t, map[string]any{"session_id": "s2"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "move to new chat")
	assert.Equal(t, "s1", f.runner(t, runnerID).CurrentSession, "the runner is where it was")
}

func TestIngestHook_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", "session_start",
		mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestIngestHook_ChatLookupFailure_ReturnsWrappedError(t *testing.T) {
	f, cs, _ := newFaultFixture(t)

	_, runnerID := f.spawn(t, "claude")

	cs.failGetChat = fmt.Errorf("boom: get chat")
	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hi"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingest hook: chat")
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

func TestListChats_GetChat_ConversationsForChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	chats, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	assert.Equal(t, chatID, chats[0].ID)

	chat, err := f.usecase.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, chatID, chat.ID)

	convs, err := f.usecase.ConversationsForChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, convs, 1)
	assert.Equal(t, "s1", convs[0].SessionID)
	assert.Equal(t, "claude", convs[0].ProviderID)

	live, err := f.usecase.LiveRunnerForChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)
}

func TestListProviders_ReturnsEveryDescriptorForTheWorkspaceHome(t *testing.T) {
	f := newFixture(t)

	descs, err := f.usecase.ListProviders(f.ctx, "ws1")
	require.NoError(t, err)
	require.NotEmpty(t, descs)

	ids := make([]string, len(descs))
	for i, d := range descs {
		ids[i] = d.ID
	}
	assert.Contains(t, ids, "claude")
	assert.Contains(t, ids, "codex")
}

func TestListProviders_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.ws.err = fmt.Errorf("boom: worktree lookup")

	descs, err := f.usecase.ListProviders(f.ctx, "ws1")
	require.Error(t, err)
	assert.Nil(t, descs)
	assert.Contains(t, err.Error(), "list providers")
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestListProviders_DescriptorEnumerationFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	// AllDescriptors unions the embedded descriptor ids with the on-disk override dir
	// under crowbar home, then loads EVERY id through ResolveDescriptor. An invalid
	// override (here: an id-less descriptor) therefore fails enumeration — the only way
	// the second branch of ListProviders can error.
	dir := filepath.Join(f.ws.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("id: \"\"\n"), 0o600))

	descs, err := f.usecase.ListProviders(f.ctx, "ws1")
	require.Error(t, err)
	assert.Nil(t, descs)
	assert.Contains(t, err.Error(), "list providers")
	assert.Contains(t, err.Error(), "enumerate descriptor")
}

// argAfter returns the argv token following flag.
func argAfter(t *testing.T, argv []string, flag string) string {
	t.Helper()
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	t.Fatalf("flag %q not found in argv %v", flag, argv)
	return ""
}

// configValue returns the value of the `-c <prefix>...` override a codex spawn was
// given. codex takes several -c flags (hooks, update check, the injected context), so
// "the token after the first -c" is not good enough.
func configValue(t *testing.T, argv []string, prefix string) string {
	t.Helper()
	for i, a := range argv {
		if a == "-c" && i+1 < len(argv) && strings.HasPrefix(argv[i+1], prefix) {
			return argv[i+1]
		}
	}
	t.Fatalf("no -c %s... override in argv %v", prefix, argv)
	return ""
}

// TestSpawnChat_MissingCLI_PropagatesCommandNotFound: when the vendor CLI is not
// installed, the sentinel must survive the usecase intact. Burying it in a generic
// wrap chain is what mapped it to an opaque 500, so the UI had nothing to say and the
// chat button just did nothing.
func TestSpawnChat_MissingCLI_PropagatesCommandNotFound(t *testing.T) {
	f := newFixture(t)
	f.term.err = fmt.Errorf("%w: claude", engineterminal.ErrCommandNotFound)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.Error(t, err)
	require.ErrorIs(t, err, engineterminal.ErrCommandNotFound,
		"the not-installed fact must reach the handler, which maps it to 424")
	assert.Contains(t, err.Error(), "claude", "and it must name the provider the UI has to report")
}
