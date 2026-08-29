package chat_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// ─── from switch_test.go ──────────────────────────────────────────────

// TestSwitchProvider_ResolvesCwdThroughTheAncestorWalkForABubble proves the
// LAST call site that still resolved a cwd straight off chat.WorkspaceID. Its
// preflight (switch.go) reads WorktreeDir before anything is torn down, to
// resolve the incoming provider's descriptor — and for a bubble that read went
// out with the empty id.
//
// It matters more now than when Task 22 fixed the spawn path: Promote respawns
// through SwitchProvider, and a bubble is the only kind of chat Promote can be
// called on at all, so this is on the promotion path by construction.
//
// The assertion is on worktreeDirIDs rather than lastWorkspaceID: the switch
// resolves a cwd twice (this preflight, then the spawn), so the LAST id is
// "ws1" whether or not the preflight was fixed. Only the whole call list tells
// them apart, and fakeWorkspace answers "" as happily as any real id — which is
// exactly how this survived a green suite.
func TestSwitchProvider_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	f.ws.worktreeDirIDs = nil

	_, err := f.usecase.SwitchProvider(f.ctx, bubbleID, "codex")
	require.NoError(t, err)
	f.wait()

	require.NotEmpty(t, f.ws.worktreeDirIDs)
	assert.NotContains(t, f.ws.worktreeDirIDs, "",
		"every cwd a bubble's switch resolves must come from its workspace-owning ancestor")
}

func TestSwitchProvider_TerminatesOutgoingCLI_AndTakesOverTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	oldTerm := f.runner(t, oldRunner).TerminalSession
	require.NotEmpty(t, oldTerm)

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, f.term.terminatedIDs(), oldTerm, "the outgoing CLI is quit gracefully")

	// The chat did not move and was not written to; only the runner on it changed.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
	assert.NotEqual(t, oldTerm, live.TerminalSession, "the incoming CLI has its own PTY")

	// The outgoing runner is still alive until its PTY actually dies — Crowbar never
	// asserts a death it has not observed.
	f.term.exit(t, oldTerm)
	f.wait()
	_, err = f.runners.Get(f.ctx, oldRunner)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "and then the PTY's death carries it away")
}

// TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch: when
// TerminateGraceful fails because the terminal session is already gone (the one error
// the real engine returns today), the switch must still proceed.
func TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
}

// TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch: a TerminateGraceful
// failure that is NOT "session already gone" must abort the switch entirely rather
// than leave two live CLIs pointed at one chat.
func TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminate outgoing terminal")

	require.Equal(t, 1, f.term.callCount(), "no second CLI may be spawned after a real terminate failure")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, oldRunner, live.ID, "the chat still belongs to the CLI that would not die")
}

// TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate: the handoff is
// assembled BEFORE the terminate, so a failure there leaves the chat completely
// untouched — rather than killing the old CLI and spawning the new one with an EMPTY
// handoff.
func TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")

	f.ws.err = errors.New("boom: worktree lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)

	f.ws.err = nil // let the assertion reads resolve the worktree again
	assert.Empty(t, f.term.terminatedIDs(), "the outgoing CLI must never be terminated when the handoff fails first")
	require.Equal(t, 1, f.term.callCount(), "and no new CLI is spawned")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, oldRunner, live.ID)
}

func TestSwitchProvider_Forward_SpawnsTargetProviderWithHandoff(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "prior turn content for handoff")

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)

	require.Equal(t, 2, f.term.callCount())
	newCall := f.term.calls[1]
	// Basename: argv[0] is binpath.Resolve'd to the CLI's absolute path when installed.
	assert.Equal(t, "codex", filepath.Base(newCall.argv[0]))
	assert.Contains(t, strings.Join(newCall.argv, "\x00"), "prior turn content for handoff")
}

// TestSwitchProvider_Broadcasts_NoChatEvent: a handoff changes which CLI is on the
// chat. The CHAT is not written to at all, so it emits no lifecycle event; the runner
// feed carries the whole story — the outgoing runner is taken OFF the chat (displaced)
// the moment we quit it, the incoming one starts, and the outgoing one exits later, when
// its PTY finally dies. The displaced frame is what tells a client the old runner no
// longer holds the chat, without waiting for a death it does not control.
func TestSwitchProvider_Broadcasts_NoChatEvent(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	oldTerm := f.runner(t, oldRunner).TerminalSession
	f.bc.reset()
	f.rbc.reset()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.term.exit(t, oldTerm)
	f.wait()

	assert.Empty(t, f.bcKinds(t), "a provider switch writes nothing to the chat aggregate")
	assert.Equal(t, []string{"displaced", "started", "exited"}, f.runnerKinds(t))
}

// TestSwitchProvider_SwitchBack_ResumesTheConversationWithSeparateArgvTokens drives
// forward+back: spawn claude, bind its conversation, switch to codex, switch back. The
// switch-back resumes claude's OWN conversation by expanding+tokenizing
// descriptor.Session.Resume.Arg ("--resume {id}") into two SEPARATE argv tokens.
func TestSwitchProvider_SwitchBack_ResumesTheConversationWithSeparateArgvTokens(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	// A conversation id is not a conversation: the CLI only WRITES one once it has said
	// something, so it is only resumable after a real turn.
	turn(t, f, runnerID, "claude", "claude said something")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	require.Equal(t, 2, f.term.callCount())

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	resumeIdx := indexOf(argv, "--resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain --resume as its own token", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-claude-native", argv[resumeIdx+1])

	assert.NotContains(t, argv, "--resume sid-claude-native")
}

// TestSwitchProvider_SwitchBack_ResumesOverAPINotTheRedundantPTY exercises the
// codex-target switch-back path. codex is api-transport, non-hotswap:
// applyAPITransport's own thread/resume call (apiconn.go) is what actually
// resumes sid-codex-native — never the redundant hooks-only PTY spawnRunner
// still forks alongside it (codex.yaml's own comment on subagent_pre explains
// why that PTY exists at all). Handing that SAME session id to the PTY too —
// natively as `resume {id}`, or as the resume context pointer, which for a
// provider whose only resume channel is a user message IS a prompt the PTY
// will act on — makes it a second writer on a thread the api connection
// already holds. codex enforces one writer per thread (a thread-writer-lock
// file, confirmed on disk): confirmed live, the native-id case crashes that
// PTY outright and the switch that looked like it succeeded silently reverts;
// confirmed live also, the pointer-without-id case doesn't crash, but the PTY
// answers the pointer as its OWN genuine first turn, landing on this chat as
// a second, disconnected "codex" conversation the api connection knows
// nothing about. nativeResumeSteps/apiOwnsResume (prompts.go, spawn.go)
// withhold both from this PTY for exactly that reason.
//
// The api connection resuming silently, with no gap handed to it either, is a
// real, separate, KNOWN gap this leaves in place — codex.yaml declares an
// inject: at: context step (thread/inject_items) for exactly this, and
// nothing calls it yet. Recorded here, not silently assumed fixed: this test
// asserts only that the switch-back is SAFE, not that codex is told what it
// missed.
func TestSwitchProvider_SwitchBack_ResumesOverAPINotTheRedundantPTY(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner := f.spawn(t, "codex")
	f.announce(t, codexRunner, "sid-codex-native")
	turn(t, f, codexRunner, "codex", "codex ledger content")

	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	// What codex misses while it is away.
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	newRunnerID, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID)

	// The api connection is what actually resumes sid-codex-native — see
	// resumeTarget/resolvePromptDelivery's launchSessionID plumbing, persisted
	// here as the new runner's own LaunchSessionID.
	assert.Equal(t, "sid-codex-native", f.runner(t, newRunnerID).LaunchSessionID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	assert.NotContains(t, argv, "resume",
		"the redundant PTY must never resume the SAME thread the api connection just did: %v", argv)
	for _, tok := range argv {
		assert.NotContains(t, tok, "[Crowbar]",
			"the resume pointer must not reach this PTY either — it would answer as an unrelated second conversation: %v", argv)
	}
}

// TestSwitchProvider_ForwardSwitch_CarriesWholeConversation is the other half of the
// gap rule: a provider that has never run in this chat has no conversation to resume
// and therefore no history at all, so it gets the ENTIRE ledger.
func TestSwitchProvider_ForwardSwitch_CarriesWholeConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said this first")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv

	// codex is new to this chat: no resume, and the context rides the silent
	// developer_instructions channel — never the positional user prompt.
	assert.Equal(t, -1, indexOf(argv, "resume"), "a provider new to the chat has no conversation to resume")

	doc := configValue(t, argv, "developer_instructions=")
	assert.Contains(t, doc, "claude said this first")
}

// TestRegression_ForwardSwitchOfALongChatCapsTheHandoffToTheRecentWindow pins the
// bug this cap fixes: before it existed, the "whole ledger" the test above
// describes was UNBOUNDED — a chat with a long-running conversation on the other
// provider dumped its entire history into the incoming CLI's context on every
// single switch. The handoff must now carry only the most recent
// defaultChatLogTurns turns, with the earliest ones pointed at through
// get_chat_log rather than replayed.
func TestRegression_ForwardSwitchOfALongChatCapsTheHandoffToTheRecentWindow(t *testing.T) {
	f := newFixture(t)

	// Discovered from the real cap rather than copied, so a future change to it
	// cannot leave this test silently under the threshold it means to exceed.
	kept, _ := agenttools.RecentHandoffWindow("probe", make([]struct{}, 100_000))
	handoffCap := len(kept)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	total := handoffCap + 5
	for i := 0; i < total; i++ {
		turn(t, f, runnerID, "claude", fmt.Sprintf("claude turn %d", i))
	}

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	argv := f.term.calls[f.term.callCount()-1].argv
	doc := configValue(t, argv, "developer_instructions=")
	assert.NotContains(t, doc, "claude turn 0",
		"the earliest turn must be trimmed once history exceeds the cap")
	assert.Contains(t, doc, fmt.Sprintf("claude turn %d", total-1),
		"the most recent turn must survive")
	assert.Contains(t, doc, "get_chat_log",
		"a trimmed handoff must point at how to read what was cut")
}

func TestSwitchProvider_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.SwitchProvider(f.ctx, "does-not-exist", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: chat")
}

// TestSwitchProvider_DormantChat_SwitchesAnyway: a chat whose CLI is gone (it exited,
// or died with the daemon) used to be a hard dead end — the pane told the user to
// "switch provider below to start a new one" while this call returned ErrNotFound, so
// the chat could never be re-entered by ANY route. A dormant chat now simply means
// there is no outgoing CLI to quit.
func TestSwitchProvider_DormantChat_SwitchesAnyway(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID)
	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
}

func TestSwitchProvider_WorkspaceReaderFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	// A workspace-reader failure surfaces wrapped, not swallowed. It now
	// surfaces at the preflight WorktreeDir read rather than the pending-prompt
	// guard: that guard's journal dir no longer resolves a workspace at all
	// (worktreepath.LedgerChatsDir, keyed by chat id — spec §1.5), so it clears
	// before the switch reaches this failing read.
	f.ws.err = errors.New("boom: workspace lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preflight worktree dir")
	assert.ErrorContains(t, err, "boom: workspace lookup")
}

func TestSwitchProvider_UnknownTargetProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
	assert.Equal(t, spawnCount, f.term.callCount(), "an invalid target must not spawn anything")
	assert.Len(t, f.term.terminatedIDs(), terminatedCount,
		"target planning must fail before the outgoing TUI is touched")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
}

// ---------------------------------------------------------------------------
// Resume
// ---------------------------------------------------------------------------

// TestResumeChat_RevivesLastProviderIntoItsOwnConversation: the CLI died (it exited, or
// the daemon restarted), and the chat must come back exactly where the user left it.
// Everything needed is in the chat's conversation history — the provider that was last
// here and the conversation it was in — so a revive is nothing more than "switch to the
// provider that was last here", which resumes into that conversation.
func TestResumeChat_RevivesLastProviderIntoItsOwnConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said something")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI dies
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "revive must bring back the provider that was last here")

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv
	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"revive must resume the CLI's own conversation, not start a blank one")

	// Nothing happened while it was gone, so it is handed no {context} document AT
	// ALL — not an empty one, and not a bare capability preamble. Its own session
	// already holds every turn, and a resume channel is allowed to be a USER MESSAGE
	// (codex's is), so a document with nothing in it that HAPPENED must not be
	// delivered. That rule is provider-independent, which is why claude's silent
	// --append-system-prompt channel is left off here too.
	for _, a := range argv {
		assert.NotEqual(t, "--append-system-prompt", a,
			"a revive with an empty gap has nothing to say and must inject nothing")
		assert.NotContains(t, a, "WHILE YOU WERE AWAY",
			"a revive with an empty gap must hand over no conversation")
		assert.NotContains(t, a, "HANDED-OFF CONTEXT",
			"a revived provider must never be re-fed the conversation it already has")
	}
}

// waitForClockTick blocks until real time.Now() has strictly advanced past its
// current instant — a real signal (the clock itself), never a guessed sleep
// duration. Needed wherever a gap boundary is a strict `>` against wall time
// (TurnsSince): two turns recorded back-to-back against the harness's
// in-memory sqlite can otherwise land on the identical instant and the second
// is silently excluded from "since", exactly the false negative this file's
// own gap tests must not be exposed to.
func waitForClockTick(t *testing.T) {
	t.Helper()
	start := time.Now()
	for time.Now().Equal(start) {
		if time.Since(start) > 2*time.Second {
			t.Fatal("real clock never advanced")
		}
	}
}

// TestSwitchProvider_ClaudeSwitchBack_ResumesAndPointsAtTheGap pins the live bug:
// a claude chat resumed into its own conversation recalled nothing that happened
// while it was away — first because --append-system-prompt is silent config a
// model already carrying its own --resume-restored history is free to
// deprioritize, and then, after that was fixed by delivering a pointer as a real
// user turn instead, because a pointer asking the model to call get_chat_log for
// the gap depends on the model actually choosing to call it, and confirmed live
// it often just doesn't. This asserts the real gap conversation rides that same
// positional channel directly, not a pointer to it.
func TestSwitchProvider_ClaudeSwitchBack_ResumesAndPointsAtTheGap(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	turn(t, f, claudeRunner, "claude", "claude ledger content")
	// The gap is drawn as started_at > this turn's own timestamp (TurnsSince):
	// without a real tick between them, codex's turn below can land on the
	// identical wall-clock instant and be silently excluded from the gap.
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")

	// What claude misses while it is away. codex's turn_stop maps
	// threadId/turn.items[type=agentMessage].text (see codex.yaml), NOT the flat
	// last_assistant_message shape turn() builds for claude — using turn() here
	// would silently extract an empty message and record nothing at all.
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunner, "codex", "turn_stop",
		mustJSON(t, map[string]any{
			"threadId": "sid-codex-native",
			"turn": map[string]any{
				"items": []any{
					map[string]any{"type": "agentMessage", "text": "codex spoke while claude was away"},
				},
			},
		})))
	f.wait()

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"switch-back must resume claude's own conversation, not start a blank one")

	msg := argv[len(argv)-1]
	assert.Contains(t, msg, "codex spoke while claude was away",
		"the real gap must ride the positional channel directly, not a pointer to fetch it later: %q", msg)
	// <system-reminder> is the tag claude's own harness uses to mark injected
	// content as context rather than instruction — kept regardless of whether
	// this carries a pointer or the real gap, since a bracket-tagged directive
	// with no such wrapper is what read to claude's own safety training as a
	// classic injection attempt in the first place.
	assert.True(t, strings.HasPrefix(msg, "<system-reminder>") && strings.HasSuffix(msg, "</system-reminder>"),
		"the gap must be wrapped as trusted context, not delivered as a bare user turn: %q", msg)
	assert.NotContains(t, msg, "claude ledger content",
		"a provider resumed into its own conversation must not be re-fed its own earlier turns")
	for _, a := range argv {
		assert.NotEqual(t, "--append-system-prompt", a,
			"the gap must ride the same positional channel a real turn does, not silent config")
	}
}

// TestResumeChat_LiveChat_IsNoop: reviving a chat whose CLI is alive must never tear
// that CLI down — it hands back the runner already on it. (Dormant is a QUERY, so
// "already live" is answerable without any flag.)
func TestResumeChat_LiveChat_IsNoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	got, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, got)
	assert.Equal(t, 1, f.term.callCount(), "a live chat must not respawn its CLI")
	assert.Empty(t, f.term.terminateRequestIDs())
}

// TestResumeChat_NoConversation_ReturnsError: a chat whose CLI never announced a
// conversation has nothing to resume into.
func TestResumeChat_NoConversation_ReturnsError(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no conversation to resume")
}

// TestResumeChat_ConversationWithNoTurns_SpawnsFreshInsteadOfResumingAPhantom is the
// regression for a bug that reached the running app: opening a chat the user had never
// sent a message in killed it outright, with claude printing
//
//	No conversation found with session ID: dc4b2ff8-…
//
// A SESSION ID IS NOT A CONVERSATION. Every CLI reports its id the instant it starts
// (that is when our session_start hook records it), but only WRITES the conversation
// once there is at least one message — so a chat that was opened and never used has an
// id pointing at nothing, and resuming it fails on startup. Crowbar records a turn from
// the very same hooks, so an empty ledger for that provider is the proof that there is
// nothing to resume: spawn fresh instead.
func TestResumeChat_ConversationWithNoTurns_SpawnsFreshInsteadOfResumingAPhantom(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	// The CLI came up and reported its conversation id — but the user never typed, so no
	// turn was ever recorded and claude never wrote this conversation to disk.
	f.announce(t, runnerID, "sid-never-persisted")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv
	assert.Equal(t, -1, indexOf(argv, "--resume"),
		"a conversation the CLI never wrote must NOT be resumed — claude dies with "+
			"\"No conversation found with session ID\"; argv was %v", argv)
	for _, a := range argv {
		assert.NotContains(t, a, "sid-never-persisted")
	}
}

// TestSwitchProvider_SwitchBackToProviderWithNoTurns_DoesNotResume: same rule on the
// switch-back path. A provider that ran in this chat but never said anything has no
// conversation to return to, so it is spawned fresh — and, having no history of its
// own, it gets the WHOLE conversation rather than a gap.
func TestSwitchProvider_SwitchBackToProviderWithNoTurns_DoesNotResume(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	// claude binds a conversation but never takes a turn.
	f.announce(t, claudeRunner, "sid-claude-empty")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	turn(t, f, codexRunner, "codex", "codex actually said something")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	assert.Equal(t, -1, indexOf(argv, "--resume"), "argv %v must not resume a conversation with no content", argv)
	// No conversation of its own → it is new to the conversation → it gets all of it.
	doc := argAfter(t, argv, "--append-system-prompt")
	assert.Contains(t, doc, "codex actually said something")
	assert.Contains(t, doc, "HANDED-OFF CONTEXT")
}

// TestSwitchProvider_CodexKeepsItsOwnHome is the regression for the worst bug in this
// feature: Crowbar used to point CODEX_HOME at a directory it owned and deleted, which
// made it the custodian of codex's SESSIONS. It duly destroyed them — leaving codex
// ended its segment, the directory went with it, and coming back resumed a thread that
// no longer existed, so the CLI died on startup ("no rollout found for thread id ...")
// and the chat could never return to codex.
//
// A provider owns its own sessions. Crowbar injects its hooks as config overrides and
// never touches codex's home, so there is nothing left for it to delete.
func TestSwitchProvider_CodexKeepsItsOwnHome(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner := f.spawn(t, "codex")
	f.announce(t, codexRunner, "sid-codex")
	turn(t, f, codexRunner, "codex", "codex said something")

	for _, call := range f.term.calls {
		for _, kv := range call.env {
			assert.False(t, strings.HasPrefix(kv, "CODEX_HOME="),
				"Crowbar must never own codex's home — its sessions live there")
		}
	}

	// Leave codex and come back: it resumes its own conversation, and Crowbar had no
	// session store to lose in between. The resume itself happens over the api
	// connection (applyAPITransport's thread/resume), never the redundant
	// hooks-only PTY — see apiOwnsResume (prompts.go) — so it is codex's OWN
	// resume, not one this test could have papered over by owning CODEX_HOME.
	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	newRunnerID, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	assert.Equal(t, "sid-codex", f.runner(t, newRunnerID).LaunchSessionID)
	require.Equal(t, 3, f.term.callCount())
	assert.NotContains(t, f.term.calls[2].argv, "resume",
		"the redundant PTY must never also resume codex's own conversation")
}

// ─── from midturn_test.go ─────────────────────────────────────────────

// switchResult is what a SwitchProvider driven on its own goroutine hands back.
type switchResult struct {
	runnerID string
	err      error
}

// TestSwitchProvider_MidTurn_WaitsForTheTurnBeforeQuittingTheOutgoingCLI is the headline
// regression, and it is a bug the user hit in production:
//
//	claude --resume → "No conversation found with session ID: <id>"
//
// A switch used to SIGTERM the outgoing CLI the instant it was asked to. A SIGTERM is
// what it is (rather than a SIGKILL) precisely so the CLI can flush its native
// transcript on the way out — but a CLI killed MID-TURN never writes a transcript at
// all. So the in-flight answer was lost, and the conversation the incoming CLI was then
// told to --resume did not exist.
//
// The fix is to let the turn finish first. This test proves it WITHOUT ANY TIMING: the
// switch runs on its own goroutine and the test blocks on whichever of two REAL signals
// arrives —
//
//	the switch announcing it is parked on the turn (the fix), or
//	TerminateGraceful being called (the bug, caught in the act, inside the fake).
//
// Exactly one of them must arrive, so there is nothing to sleep for and nothing to poll.
func TestSwitchProvider_MidTurn_WaitsForTheTurnBeforeQuittingTheOutgoingCLI(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.announce(t, runnerID, "sid-claude-native")

	// The user typed: the CLI is now mid-answer, and this is the state the whole bug
	// lives in.
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	killed := terminateSignal(f)
	parked := parkedOnTurn(t)

	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()

	select {
	case <-parked:
	case sess := <-killed:
		t.Fatalf("the outgoing CLI (%s) was terminated while its turn was still running: "+
			"its answer is lost and the transcript it never flushed cannot be --resume'd", sess)
	case r := <-done:
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	// Still nobody has been killed, and no incoming CLI has been spawned: the switch is
	// parked, holding its chat's spawn gate. The hook path is NEVER gated, which is what
	// lets the very next line reach the usecase at all — if it could not, this test would
	// deadlock, and that deadlock is the one this design has to be safe from.
	assert.Empty(t, f.term.terminatedIDs(), "nothing may be killed while the turn is in flight")
	assert.Equal(t, 1, f.term.callCount(), "and no incoming CLI may be spawned yet")

	// The turn completes.
	turn(t, f, runnerID, "claude", "the answer the user was waiting for")

	got := <-done
	require.NoError(t, got.err)
	f.wait()

	assert.Contains(t, f.term.terminatedIDs(), oldTerm, "and only THEN is the outgoing CLI quit")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, live.ID, "the switch then proceeds")
	assert.Equal(t, "codex", live.ProviderID)

	// The other half of the bug: the in-flight answer must not be lost. The ledger is read
	// AFTER the turn landed in it, so the incoming CLI is handed the reply the outgoing one
	// was still writing when the user clicked switch.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, strings.Join(f.term.calls[1].argv, "\x00"), "the answer the user was waiting for",
		"the handoff must carry the turn the switch waited for")
}

// TestSwitchProvider_Idle_SwitchesImmediately: the common case. A chat that is not
// working has no turn to wait for, so the switch must not park — the happy path pays
// nothing for the mid-turn guarantee.
func TestSwitchProvider_Idle_SwitchesImmediately(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "a question")
	turn(t, f, runnerID, "claude", "an answer")
	require.False(t, f.chat(t, chatID).Working, "precondition: the chat is idle")

	parked := parkedOnTurn(t)

	// Straight-line: no goroutine, no signal to wait for. If the switch parked, this call
	// would never return.
	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	select {
	case <-parked:
		t.Fatal("an idle chat has no in-flight turn: the switch must not wait for one")
	default:
	}

	assert.Contains(t, f.term.terminatedIDs(), oldTerm)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
}

// TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged: the wait is
// bounded by the CALLER'S CONTEXT and by nothing else — no timeout, no deadline, no
// clock. If the request goes away, the switch aborts exactly as every other pre-terminate
// failure aborts it: the outgoing CLI is still alive, still on its chat, still mid-turn,
// and no incoming CLI was ever spawned. Half-doing it is the one outcome that is not
// allowed.
func TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	ctx, cancel := context.WithCancel(context.Background())
	killed := terminateSignal(f)
	parked := parkedOnTurn(t)

	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()

	select {
	case <-parked:
	case sess := <-killed:
		cancel()
		t.Fatalf("the outgoing CLI (%s) was terminated mid-turn", sess)
	case r := <-done:
		cancel()
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	cancel()

	got := <-done
	require.Error(t, got.err)
	assert.ErrorIs(t, got.err, context.Canceled)
	assert.Empty(t, got.runnerID)

	f.wait()
	assert.Empty(t, f.term.terminatedIDs(), "an aborted switch kills nothing")
	assert.Equal(t, 1, f.term.callCount(), "and spawns nothing")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID, "the outgoing CLI is still on its chat")
	assert.True(t, f.chat(t, chatID).Working, "and still mid-turn")
}

// TestSwitchProvider_MidTurn_OutgoingCLIDies_ReleasesTheSwitch: the turn's completion is
// not the only way it can END. A CLI that falls over mid-answer never sends its turn_stop
// hook — so if the wait listened for that hook alone, a switch onto a crashed CLI would
// hang until the user's request timed out. The PTY's death is the other real signal, and
// it releases the wait too (the chat is simply dormant by the time we get there).
func TestSwitchProvider_MidTurn_OutgoingCLIDies_ReleasesTheSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "think hard about this")

	killed := terminateSignal(f)
	parked := parkedOnTurn(t)

	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()

	select {
	case <-parked:
	case sess := <-killed:
		t.Fatalf("the outgoing CLI (%s) was terminated mid-turn", sess)
	case r := <-done:
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	// The CLI crashes mid-answer.
	f.term.exit(t, oldTerm)

	got := <-done
	require.NoError(t, got.err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, live.ID)
	assert.Equal(t, "codex", live.ProviderID, "the switch completes onto the dead CLI's chat")
	assert.NotEqual(t, runnerID, live.ID)
}

// TestResumeChat_DormantChat_DoesNotWait: ResumeChat shares switchProviderLocked, and a
// dormant chat has no CLI and therefore no turn anybody could still be running — even
// though its Working flag may still be set (a chat whose CLI died mid-turn). The revive
// must not wait for a turn nothing is running.
func TestResumeChat_DormantChat_DoesNotWait(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	prompt(t, f, runnerID, "claude", "a question")
	turn(t, f, runnerID, "claude", "an answer")

	// It dies mid-turn: nothing is running, and nothing will ever close this turn.
	prompt(t, f, runnerID, "claude", "and one more thing")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	parked := parkedOnTurn(t)

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	select {
	case <-parked:
		t.Fatal("a dormant chat has nothing running: a revive must never wait for a turn")
	default:
	}

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID)
}

// prompt drives a user_prompt hook: the user submitting a message, which is what OPENS a
// turn (the chat goes Working) in production. Every mid-turn test starts here, because
// this hook is the only thing that ever puts a chat mid-turn.
func prompt(
	t *testing.T,
	f testFixture,
	runnerID, provider, message string,
) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, "user_prompt",
		mustJSON(t, map[string]any{"prompt": message})))
	f.wait()
}

// terminateSignal reports the id of the FIRST terminal session TerminateGraceful is
// called with, as it is called. A test blocks on it to catch a kill in the act.
func terminateSignal(
	f testFixture,
) <-chan string {
	ch := make(chan string, 1)
	f.term.duringTerminate = func(sessionID string) {
		select {
		case ch <- sessionID:
		default:
		}
	}
	return ch
}

// parkedOnTurn returns a channel closed the moment a switch parks on an in-flight turn.
// It is a REAL signal — the usecase's own log record, emitted immediately before it
// blocks — and it is what lets these tests prove a negative ("the CLI was not killed
// while the turn ran") without a single sleep: the assertion is made at a moment the test
// KNOWS the switch has reached, not one it hopes it has.
func parkedOnTurn(
	t *testing.T,
) <-chan struct{} {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := &parkHandler{ch: make(chan struct{})}
	slog.SetDefault(slog.New(h))
	return h.ch
}

type parkHandler struct {
	once sync.Once
	ch   chan struct{}
}

func (h *parkHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *parkHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, agentusecase.WaitingForTurnLog) {
		h.once.Do(func() { close(h.ch) })
	}
	return nil
}

func (h *parkHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *parkHandler) WithGroup(_ string) slog.Handler { return h }

// TestRegression_ClearMidTurn_ClosesTheTurnOnTheChatLeftBehind is the workspace-spinner
// wedge, reported from production as "all the agents stopped working but the workspace
// is still spinning".
//
// A turn is opened by the user_prompt hook and closed by the turn_stop hook, and BOTH
// are filed against the chat the runner is on WHEN THEY ARRIVE. So a /clear taken
// mid-turn splits the pair: user_prompt landed on the old chat, the runner then walks
// into a freshly minted one, and the turn_stop lands over there. Nothing closes the
// turn where it was opened.
//
// The cost is not cosmetic. domain.Chat.Working stays true forever, and the
// workspace's derived Working overlay (repositories.Container.agentWorking) keeps that
// chat in its mid-turn set for the life of the daemon — so the sidebar and context-pill
// spinners run forever over a workspace where nothing at all is happening. The chat ROWS
// look idle the whole time, because the frontend clears its own working map on every
// reseed while the daemon-side set has no such reset. That asymmetry is exactly what the
// bug report described.
//
// The runner has left, and no successor has taken the old chat, so closeAbandonedTurn's
// own guards are the whole judgement: it asserts nothing about any live process.
func TestRegression_ClearMidTurn_ClosesTheTurnOnTheChatLeftBehind(t *testing.T) {
	f := newFixture(t)

	left, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	// The user typed, and then hit /clear before the answer came back.
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, left).Working, "precondition: the chat is mid-turn")

	f.announce(t, runnerID, "sid-after-clear") // /clear: mints a chat, moves the runner
	f.wait()

	entered := f.runner(t, runnerID).CurrentChatID
	require.NotEqual(t, left, entered, "precondition: the /clear moved the runner off")

	assert.False(t, f.chat(t, left).Working,
		"the chat the runner walked out of must not be left mid-turn: its turn_stop is "+
			"going to land on the new chat, so nothing else will ever close it here")
	assert.Nil(t, f.chat(t, left).CurrentTurnStarted, "a closed turn must clear CurrentTurnStarted")
}

// ─── from race_test.go ────────────────────────────────────────────────

// TestSwitchProvider_LostStartRace_TearsDownOrphanCLI is the retarget of the old
// keyed_mutex / OpenSegment-race test. Both are gone: per-aggregate concurrency is the
// asynx write-path (id,version) optimistic-concurrency control, and there is no longer
// an OpenSegment that can be rejected because "an active segment already exists" — a
// chat holds no process state to conflict over.
//
// What the USECASE still owns, and what this pins deterministically, is the orphan
// teardown. A pure command cannot spawn a process, so the CLI is necessarily live
// BEFORE the runner that describes it can be recorded; if that record fails for any
// reason, the just-spawned CLI must be torn down. A running CLI that nothing in Crowbar
// points at is invisible — it is the state the whole refactor exists to make
// impossible — so the teardown is unconditional, and the original error still surfaces
// (an ErrValidation still classifies as a conflict upstream). We force the failure with
// a store double rather than a timing-dependent goroutine storm: no sleeps, no
// nondeterminism.
func TestSwitchProvider_LostStartRace_TearsDownOrphanCLI(t *testing.T) {
	f, _, rs := newFaultFixture(t)

	chatID, _ := f.spawn(t, "claude")

	rs.failStart = fmt.Errorf("start runner: %w", asynxModels.ErrValidation)

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation,
		"a lost race must surface as a conflict (ErrValidation), not a bare failure")

	// Two CLIs were spawned: term-1 (the original claude) and term-2 (the target codex
	// whose runner could not be recorded). term-2 must have been torn down so no orphan
	// process leaks; term-1 was terminated as the normal outgoing-CLI quit.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-2",
		"the just-spawned CLI whose runner could not be recorded must be torn down")
}

// ─── from spawn_startup_exit_test.go ──────────────────────────────────

func exitDuringFork(f testFixture) {
	f.term.duringForkCall = func(c commandCall) { c.onExit() }
}

func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsIsRefused(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup,
		"a CLI that died before its runner row existed has not started")
	assert.Contains(t, err.Error(), "exited during startup",
		"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
}

func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsLeavesNoChat(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup)
	f.wait()

	chats, listErr := f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	assert.Empty(t, chats, "a refused spawn must not leave a chat behind")

	f.term.duringForkCall = nil
	chatID, _ := f.spawn(t, "claude")
	chats, listErr = f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	require.Len(t, chats, 1, "a working provider still creates exactly one chat")
	assert.Equal(t, chatID, chats[0].ID)
}

// ─── from catalog_test.go ─────────────────────────────────────────────

func TestSlashCatalogRejectsResultWhenLiveRunnerChangesDuringProbe(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))

	// Two FIFOs, not two files. Opening one end of a FIFO blocks until the other end
	// is opened, which makes each handoff with the helper PROCESS a real signal —
	// there is no interval to pick and nothing to re-sample, so the test cannot be
	// wrong about how fast this machine is.
	started := filepath.Join(t.TempDir(), "started")
	release := filepath.Join(t.TempDir(), "release")
	require.NoError(t, syscall.Mkfifo(started, 0o600))
	require.NoError(t, syscall.Mkfifo(release, 0o600))
	t.Setenv("CROWBAR_CATALOG_HELPER_STARTED", started)
	t.Setenv("CROWBAR_CATALOG_HELPER_RELEASE", release)
	// If the test fails before it releases, the helper is parked on the read end.
	// O_NONBLOCK returns ENXIO instead of blocking when it has already exited, so
	// cleanup can never be the thing that hangs.
	t.Cleanup(func() {
		if w, err := os.OpenFile(release, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = w.Close()
		}
	})

	descriptor := fmt.Sprintf(`
id: codex
spawn:
  cmd: %q
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
presentation:
  slash_catalog:
    completeness: complete
    timeout_ms: 5000
    max_stdout_bytes: 1048576
    max_stderr_bytes: 65536
    max_items: 20
    pipeline:
      adapter: json_text_section
      command: ["-test.run=^TestSlashCatalogPlacementHelperProcess$"]
      text_path: "[].content[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+): (?P<description>.*)$'
      item:
        label: "{name}"
        description: "{description}"
        insert_text: "${name} "
        source: "test"
`, os.Args[0])
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(descriptor), 0o600,
	))

	chatID, runnerID := f.spawn(t, "codex")
	probeDone := make(chan error, 1)
	go func() {
		_, err := f.usecase.SlashCatalog(f.ctx, chatID)
		probeDone <- err
	}()

	inFlight, err := os.Open(started)
	require.NoError(t, err, "the deterministic provider command must be in flight")
	require.NoError(t, inFlight.Close())

	_, exitErr := f.runners.Exit(f.ctx, runnerID, time.Now())
	require.NoError(t, exitErr)
	f.wait()
	releaser, err := os.OpenFile(release, os.O_WRONLY, 0)
	require.NoError(t, err)
	require.NoError(t, releaser.Close())

	select {
	case err := <-probeDone:
		require.ErrorIs(t, err, agentusecase.ErrSlashCatalogSuperseded,
			"a result from a TUI that no longer holds the chat must never be returned")
	case <-time.After(3 * time.Second):
		t.Fatal("slash catalog did not finish after releasing its provider command")
	}
}

func TestSlashCatalogPlacementHelperProcess(t *testing.T) {
	started := os.Getenv("CROWBAR_CATALOG_HELPER_STARTED")
	release := os.Getenv("CROWBAR_CATALOG_HELPER_RELEASE")
	if started == "" || release == "" {
		return
	}
	// Announce: the write end blocks until the parent opens the read end.
	announce, err := os.OpenFile(started, os.O_WRONLY, 0)
	if err != nil {
		os.Exit(2)
	}
	_ = announce.Close()
	// Park: the read end blocks until the parent opens the write end.
	parked, err := os.Open(release)
	if err != nil {
		os.Exit(3)
	}
	_ = parked.Close()
	_, _ = fmt.Fprint(os.Stdout, `[{"content":[{"text":"<skills>\n- current: Current skill\n</skills>"}]}]`)
	os.Exit(0)
}

func TestSlashCatalog_MapsEveryEngineFailureToItsOwnAppError(t *testing.T) {
	testCases := []struct {
		name    string
		command string
		want    error
	}{
		{
			name:    "provider command unavailable",
			command: `"/nonexistent/crowbar-not-a-real-cli"`,
			want:    agentusecase.ErrSlashCatalogUnavailable,
		},
		{
			name:    "malformed output",
			command: `"/usr/bin/true"`,
			want:    agentusecase.ErrSlashCatalogMalformed,
		},
		{
			name:    "command failed",
			command: `"/usr/bin/false"`,
			want:    agentusecase.ErrSlashCatalogCommand,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
			require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
			writeCatalogDescriptor(t, f, tc.command)

			chatID, _ := f.spawn(t, "codex")

			_, err := f.usecase.SlashCatalog(f.ctx, chatID)

			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestSlashCatalog_UnsupportedWhenTheProviderDeclaresNoCatalogue(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(`
id: codex
spawn:
  cmd: /usr/bin/true
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
`), 0o600,
	))

	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogUnsupported)
}

func TestSlashCatalog_RefusesAChatWithNoLiveCLI(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	_, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogNoLiveTUI)
}

// TestSlashCatalog_ResolvesCwdThroughTheAncestorWalkForABubble proves
// SlashCatalog (catalog.go) resolves a bubble's cwd through the SAME
// ancestor walk spawnPaths uses (Task 22) — a genuinely separate call site
// that used to resolve WorktreeDir from chat.WorkspaceID directly.
//
// The assertion is on f.ws.lastWorkspaceID, not merely on the call
// succeeding: fakeWorkspace answers every id identically, including "",
// which is exactly how the original bug went undetected through 21 tasks
// (see spawnPaths' own test coverage history). Only checking WHICH id
// reached the fake proves the fallback actually ran.
func TestSlashCatalog_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")

	_, err := f.usecase.SlashCatalog(f.ctx, bubbleID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogUnsupported,
		"must reach the provider's OWN missing-capability refusal, not a cwd-resolution failure")
	assert.Equal(t, "ws1", f.ws.lastWorkspaceID,
		"must resolve the bubble's cwd through its workspace-owning ancestor, not its own empty WorkspaceID")
}

func writeCatalogDescriptor(t *testing.T, f testFixture, command string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(fmt.Sprintf(`
id: codex
spawn:
  cmd: %s
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
presentation:
  slash_catalog:
    completeness: complete
    timeout_ms: 5000
    pipeline:
      adapter: json_text_section
      command: ["--version"]
      text_path: "[].content[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+)$'
      item:
        label: "{name}"
        insert_text: "${name} "
        source: "test"
`, command)), 0o600,
	))
}

// ─── from prompts_test.go ─────────────────────────────────────────────

func TestSubmitPrompt_RejectsNULBeforeJournalOrTUITeardown(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "invalid\x00argv", uuid.NewString())
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Equal(t, spawnCount, f.term.callCount(), "invalid input must not start a replacement")
	assert.Len(t, f.term.terminatedIDs(), terminatedCount, "invalid input must not touch the outgoing TUI")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
	_, statErr := os.Stat(filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID, "prompt-requests"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "validation must precede durable dispatch intent")
}

func TestSubmitPrompt_ParentDirectorySyncFailureAbortsBeforeTUITeardown(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())
	agentusecase.SetPromptJournalDirSync(f.usecase.RunnerUsecase, func(string) error {
		return errors.New("injected parent fsync failure")
	})

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "must be durable first", uuid.NewString())
	require.Error(t, err)
	assert.NotErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown,
		"the replacement process was never attempted, so this is not an unknown delivery")
	assert.Equal(t, spawnCount, f.term.callCount())
	assert.Len(t, f.term.terminatedIDs(), terminatedCount)
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID, "durability failure must leave the outgoing TUI untouched")
}

func TestSubmitPrompt_FreshLazyCodexNeedsNoBoundSession(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	message := "FIRST REACT MESSAGE"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	require.NotEmpty(t, result.RunnerID)
	require.NotEmpty(t, result.TerminalSessionID)

	call := f.term.calls[f.term.callCount()-1]
	assert.NotContains(t, call.argv, "resume", "a lazy TUI with no announced session is a safe fresh start")
	assert.Equal(t, message, call.argv[len(call.argv)-1], "the completed prompt is one final argv element")
}

func TestSubmitPrompt_ResumeCodexOrdersSubcommandSessionThenPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "native-session")
	turn(t, f, runnerID, "codex", "the current conversation exists")
	message := "CONTINUE FROM REACT"

	// codex is api-transport, non-hotswap: the redundant hooks-only PTY this
	// restart still forks must never ALSO resume native-session natively — see
	// apiOwnsResume (prompts.go) — so no `resume {id} --` prefix precedes the
	// message; native-session survives only as this replacement runner's own
	// LaunchSessionID.
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	assert.Equal(t, "native-session", f.runner(t, submission.RunnerID).LaunchSessionID)
	call := f.term.calls[f.term.callCount()-1]
	assert.NotContains(t, call.argv, "resume")
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_FreshClaudeTerminatesVariadicMCPBeforeFinalPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	message := "CLAUDE REACT MESSAGE"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	mcpAt := indexOf(call.argv, "--mcp-config")
	contextAt := indexOf(call.argv, "--append-system-prompt")
	require.GreaterOrEqual(t, mcpAt, 0)
	require.Greater(t, contextAt, mcpAt+1,
		"a following option terminates Claude's variadic --mcp-config before the positional prompt")
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_BlocksNextDispatchUntilUserPromptHook(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", uuid.NewString())
	require.NoError(t, err)
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "two", uuid.NewString())
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"spawn success precedes Working=true; the durable pending request closes that no-hook window")
}

func TestSubmitPrompt_MatchingLateHookFromOutgoingRunnerDoesNotConfirmNewDispatch(t *testing.T) {
	f := newFixture(t)
	chatID, outgoingID := f.spawn(t, "codex")
	f.announce(t, outgoingID, "old-session")
	message := "same text"

	hookDone := make(chan error, 1)
	f.term.duringTerminate = func(string) {
		go func() {
			hookDone <- f.usecase.IngestHook(f.ctx, outgoingID, "codex", "user_prompt",
				mustJSON(t, map[string]any{"prompt": message, "session_id": "old-session"}))
		}()
	}
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()
	f.term.duringTerminate = nil

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString())
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"the old runner's matching hook must not clear the replacement's pending-delivery barrier")
}

func TestSubmitPrompt_ReplacementSpawnFailureStaysOutcomeUnknown(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	f.term.err = errors.New("replacement create failed")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown,
		"a non-command-not-found CreateCommand error may follow a successful fork")
	record, readErr := os.ReadFile(filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID, "prompt-requests", requestID+".json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(record), `"state":"uncertain"`,
		"returning outcome_unknown must release the durable dispatching barrier")

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	assert.ErrorIs(t, retryErr, agentusecase.ErrPromptOutcomeUnknown,
		"outgoing displacement must not mark the blank-runner dispatch safely failed")
}

func TestSubmitPrompt_ReplacementExitBeforeHookStaysOutcomeUnknown(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	require.NoError(t, err)
	f.term.exit(t, result.TerminalSessionID)
	f.wait()

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	assert.ErrorIs(t, retryErr, agentusecase.ErrPromptOutcomeUnknown,
		"process exit can race a hook already in flight, so retrying must not duplicate the prompt")
}

func TestSubmitPrompt_RunnerPersistFailureAfterPTYStartIsOutcomeUnknown(t *testing.T) {
	f, _, runners := newFaultFixture(t)
	chatID, _ := f.spawn(t, "codex")
	runners.failStart = errors.New("runner persistence failed after fork")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", uuid.NewString())
	assert.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	assert.Equal(t, 2, f.term.callCount(), "the replacement PTY started before runner persistence failed")
}

func TestSubmitPrompt_RunnerLookupFailureAndAcceptedCrashGapAreSafe(t *testing.T) {
	f, _, runners := newFaultFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	message := "accepted before response bookkeeping"

	runners.afterStart = func() {
		runners.afterStart = nil
		replacement, err := f.runners.LiveRunnerForChat(f.ctx, chatID)
		require.NoError(t, err)
		require.NoError(t, f.usecase.IngestHook(f.ctx, replacement.ID, "codex", "user_prompt",
			mustJSON(t, map[string]any{"prompt": message})))

		runners.failGet = errors.New("post-spawn runner lookup failed")

		runners.failGetAfter = 2
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	runners.failGet = nil

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
	assert.ErrorIs(t, retryErr, agentusecase.ErrPromptAlreadyAccepted,
		"accepted-with-runner but without a committed terminal id must never return a blank success DTO")
}

func TestSubmitPrompt_JournalResultCommitFailureIsOutcomeUnknownAndDoesNotWedgeNewIDs(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	journalDir := filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID, "prompt-requests")
	blockedDir := journalDir + ".blocked"
	f.term.duringFork = func() {
		require.NoError(t, os.Rename(journalDir, blockedDir))
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "commit gap", uuid.NewString())
	f.term.duringFork = nil
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	require.NoError(t, os.Rename(blockedDir, journalDir))

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, "deliberate follow-up", uuid.NewString())
	require.NoError(t, retryErr)
}

func TestReconcileRunnersOnBoot_MarksBlankDispatchIntentUncertain(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	journalDir := filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID, "prompt-requests")
	blockedDir := journalDir + ".blocked"
	f.term.duringFork = func() {
		require.NoError(t, os.Rename(journalDir, blockedDir))
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "crash gap", requestID)
	f.term.duringFork = nil
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	require.NoError(t, os.Rename(blockedDir, journalDir))
	recordPath := filepath.Join(journalDir, requestID+".json")
	before, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	assert.Contains(t, string(before), `"state":"dispatching"`)

	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	after, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	assert.Contains(t, string(after), `"state":"uncertain"`,
		"boot must durably release a crash-orphan dispatch barrier")
}

func TestSubmitPrompt_CompletedStoppedResumedChatKeepsNativeResumeIdentity(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "durable-session")
	turn(t, f, runnerID, "codex", "completed before the TUI stopped")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, "term-1")
	f.wait()
	resumedID, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "durable-session", f.runner(t, resumedID).LaunchSessionID)
	f.announce(t, resumedID, "durable-session")

	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, "continue after reopen", uuid.NewString())
	require.NoError(t, err)
	// codex is api-transport, non-hotswap: this restart's redundant hooks-only
	// PTY must never ALSO resume durable-session natively (apiOwnsResume,
	// prompts.go) — launch-as-resume identity survives instead as this
	// replacement runner's own LaunchSessionID, even though old ledger turns
	// predate the new runner's session_start.
	assert.Equal(t, "durable-session", f.runner(t, submission.RunnerID).LaunchSessionID)
	assert.NotContains(t, f.term.calls[f.term.callCount()-1].argv, "resume")
}

func TestSubmitPrompt_NativeTUIResumeOfKnownSessionKeepsContext(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "known-session")
	turn(t, f, runnerID, "codex", "completed in the known conversation")

	f.announce(t, runnerID, "temporary-new-session")
	f.announce(t, runnerID, "known-session")
	current := f.runner(t, runnerID)
	require.Equal(t, chatID, current.CurrentChatID)
	require.True(t, current.CurrentSessionResumable)

	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, "continue immediately after native resume", uuid.NewString())
	require.NoError(t, err)
	// codex is api-transport, non-hotswap: this restart's redundant hooks-only
	// PTY must never ALSO resume known-session natively (apiOwnsResume,
	// prompts.go) — the identity survives instead as this replacement
	// runner's own LaunchSessionID.
	assert.Equal(t, "known-session", f.runner(t, submission.RunnerID).LaunchSessionID)
	assert.NotContains(t, f.term.calls[f.term.callCount()-1].argv, "resume")
}

// TestSubmitPrompt_VirginNativeSessionAfterSwitchCarriesTheFullHandoff pins the
// exact bug a live user hit: switching providers spawns the new CLI with the
// whole conversation correctly injected on its silent placeholder — but every
// restart_tui provider (see claude.yaml/codex.yaml) delivers EVERY message,
// including the first, by killing that placeholder and respawning with the
// message baked in. Before this fix, that replacement spawn hardcoded an
// empty conversation and zero gap regardless of history, discarding the
// handoff the instant the user typed anything: the new provider answered as
// if it had never been told what came before.
func TestSubmitPrompt_VirginNativeSessionAfterSwitchCarriesTheFullHandoff(t *testing.T) {
	f := newFixture(t)
	chatID, codexRunnerID := f.spawn(t, "codex")
	f.announce(t, codexRunnerID, "codex-session")
	// codex's turn_stop maps threadId/turn.items[type=agentMessage].text (see
	// codex.yaml), NOT claude's flat last_assistant_message shape that turn()
	// builds — using turn() here would silently extract an empty message.
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunnerID, "codex", "turn_stop",
		mustJSON(t, map[string]any{
			"threadId": "codex-session",
			"turn": map[string]any{
				"items": []any{
					map[string]any{"type": "agentMessage", "text": "I like turtles"},
				},
			},
		})))
	f.wait()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	claudeRunner, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	// The switch's own placeholder is silent — claude reports its OWN session
	// id the moment it starts, well before the user has said anything to it.
	f.announce(t, claudeRunner.ID, "claude-session")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I say before?", uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	contextAt := indexOf(call.argv, "--append-system-prompt")
	require.GreaterOrEqual(t, contextAt, 0, "the replacement spawn must still carry the handoff")
	require.Less(t, contextAt+1, len(call.argv))
	assert.Contains(t, call.argv[contextAt+1], "I like turtles",
		"the whole prior conversation, not an empty gap, since this native session never turned")
}

// TestSubmitPrompt_AlreadyTurnedNativeSessionStaysOnTheCheapPath is the
// companion regression: a session that has recorded a turn already holds its
// own history, so the fix above must not fire and re-inject a handoff on
// every ordinary follow-up message.
func TestSubmitPrompt_AlreadyTurnedNativeSessionStaysOnTheCheapPath(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "claude-session")
	turn(t, f, runnerID, "claude", "first answer")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "a normal follow-up", uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, -1, indexOf(call.argv, "--append-system-prompt"),
		"an already-turned session must not be re-handed a handoff on every follow-up")
}

func TestStartupHookBarrier_ReplaysPromptThatFiresBeforeRunnerPersistence(t *testing.T) {
	f := newFixture(t)
	message := "provider fired before runner persistence"
	var earlyRunnerID string
	f.term.duringForkCall = func(call commandCall) {
		earlyRunnerID = segmentIDFromCommand(t, call.argv)
		_, err := f.runners.Get(f.ctx, earlyRunnerID)
		require.Error(t, err, "precondition: the fork callback runs before recordRunner")
		require.NoError(t, f.usecase.IngestHook(f.ctx, earlyRunnerID, "codex", "user_prompt",
			mustJSON(t, map[string]any{"prompt": message})))
	}

	chatID, runnerID := f.spawn(t, "codex")
	f.term.duringForkCall = nil
	assert.Equal(t, runnerID, earlyRunnerID)
	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "user", page.Items[0].Role)
	assert.Equal(t, message, page.Items[0].Text)
	assert.True(t, f.chat(t, chatID).Working)
}

func TestSwitchProvider_DoesNotKillPromptAwaitingAcceptanceFromAnotherWindow(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "queued elsewhere", uuid.NewString())
	require.NoError(t, err)
	spawnCount := f.term.callCount()
	terminated := len(f.term.terminatedIDs())

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	assert.Equal(t, spawnCount, f.term.callCount())
	assert.Len(t, f.term.terminatedIDs(), terminated,
		"the replacement TUI awaiting its user_prompt hook must remain alive")
}

func segmentIDFromCommand(t *testing.T, argv []string) string {
	t.Helper()
	const marker = "--segment "
	joined := strings.Join(argv, "\n")
	start := strings.Index(joined, marker)
	require.GreaterOrEqual(t, start, 0, "rendered hook command must carry the runner id")
	fields := strings.Fields(joined[start+len(marker):])
	require.NotEmpty(t, fields)
	return strings.Trim(fields[0], `"'`)
}

func TestSubmitPrompt_ExitAfterStartupBarrierBeforeJournalCommitIsUncertain(t *testing.T) {
	f, _, runners := newFaultFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	runners.afterGet = func(replacement agents.Runner) {
		runners.afterGet = nil

		f.term.exit(t, replacement.TerminalSession)
		f.wait()
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "exit in commit gap", requestID)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	record, readErr := os.ReadFile(filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID, "prompt-requests", requestID+".json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(record), `"state":"uncertain"`,
		"the pre-journaled runner id lets onExit correlate before markSpawned")
}

func TestSubmitPrompt_IdempotentRetryReturnsOriginalSpawnWhilePending(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID)
	require.NoError(t, err)
	retry, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID)
	require.NoError(t, err)
	assert.Equal(t, first, retry)
	assert.Equal(t, 2, f.term.callCount(), "the retry must not spawn a third provider TUI")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "different operation", requestID)
	assert.ErrorIs(t, err, agentusecase.ErrPromptRequestIDConflict)
}

func TestSubmitPrompt_ConcurrentSameRequestIDDeliversOnce(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	spawnsBefore := f.term.callCount()

	requestID := uuid.NewString()
	const message = "deliver me exactly once"

	type outcome struct {
		dto domain.AgentPromptSubmission
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			d, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
			results <- outcome{dto: d, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	f.wait()

	spawned := f.term.callCount() - spawnsBefore
	assert.LessOrEqual(t, spawned, 1,
		"one request id must never start two replacement CLIs (spawned %d)", spawned)

	if first.err == nil && second.err == nil {
		assert.Equal(t, first.dto, second.dto,
			"an idempotent retry must return the ORIGINAL delivery, not a second one")
	}
}

func TestSubmitPrompt_RejectsBadInputBeforeTouchingAnything(t *testing.T) {
	testCases := []struct {
		name    string
		text    string
		request string
	}{
		{"empty text", "", uuid.NewString()},
		{"whitespace only", "   \n\t ", uuid.NewString()},
		{"oversized text", strings.Repeat("x", 1<<20), uuid.NewString()},
		{"request id is not a uuid", "hello", "not-a-uuid"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, "codex")
			spawns := f.term.callCount()
			terminated := len(f.term.terminatedIDs())

			_, err := f.usecase.SubmitPrompt(f.ctx, chatID, tc.text, tc.request)

			require.ErrorIs(t, err, apperr.ErrInvalidArgument)
			assert.Equal(t, spawns, f.term.callCount(), "no replacement was started")
			assert.Len(t, f.term.terminatedIDs(), terminated, "the live CLI was not touched")
			live, liveErr := f.liveRunnerFor(t, chatID)
			require.NoError(t, liveErr)
			assert.Equal(t, runnerID, live.ID)
		})
	}
}

func TestSubmitPrompt_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.SubmitPrompt(f.ctx, uuid.NewString(), "hello", uuid.NewString())

	require.Error(t, err)
}

func TestSubmitPrompt_RefusesADormantChat(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptSessionUnavailable)
}

func TestSubmitPrompt_RefusesAProviderWithNoDeclaredDelivery(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(`
id: codex
spawn:
  cmd: /usr/bin/true
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
`), 0o600,
	))
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

func TestRegression_EveryShippedProviderDeliversAPromptByReplacingTheCLI(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, provider)
			turn(t, f, runnerID, provider, "a turn ended")
			spawns := f.term.callCount()
			message := "deliver me by restart"

			result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
			require.NoError(t, err)

			require.Equal(t, spawns+1, f.term.callCount(),
				"the message is carried by a process that did not exist before it")
			require.NotEqual(t, runnerID, result.RunnerID,
				"the replacement is a new runner: the CLI holding the chat was replaced")
			call := f.term.calls[f.term.callCount()-1]
			assert.Equal(t, message, call.argv[len(call.argv)-1],
				"the prompt is the final argv element of the replacement")

			require.NoError(t, f.usecase.IngestHook(f.ctx, result.RunnerID, provider, "user_prompt",
				mustJSON(t, map[string]any{"prompt": message})))
			f.wait()

			page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
			require.NoError(t, err)
			require.NotEmpty(t, page.Items)
			last := page.Items[len(page.Items)-1]
			assert.Equal(t, domain.TurnRoleUser, last.Role)
			assert.Equal(t, message, last.Text, "recorded verbatim, with no wrapper to strip")
		})
	}
}

// ─── from compact_test.go ─────────────────────────────────────────────

// compactingDescriptorBody declares the compaction gesture the way claude does: no API
// for it, so the trigger is the slash command injected over the prompt transport.
const compactingDescriptorBody = `
id: claude
display_name: Compacting
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
  compact_start:
    out: prompt
    send:
      text: "/compact"
runtime:
  transport: hooks
  hooks:
    format: json
`

// The same provider WITHOUT the gesture: everything else identical, so a difference in
// behaviour can only come from compact_start's absence.
const nonCompactingDescriptorBody = `
id: claude
display_name: NotCompacting
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

// Compaction reaches the CLI as the provider's OWN gesture — Crowbar cannot compact
// anything itself, the context belongs to the provider.
func TestCompact_SendsTheProvidersDeclaredGesture(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))

	call := f.term.calls[f.term.callCount()-1]
	joined := strings.Join(call.argv, " ")
	assert.Contains(t, joined, "/compact",
		"the provider's declared gesture must reach the CLI verbatim")
}

// Key-presence IS the capability. A provider that declares no gesture cannot be asked,
// and must say so rather than silently doing nothing — a compact button that appears
// to work and does not is worse than one that is absent.
func TestCompact_AProviderWithNoGestureIsNotFound(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	before := f.term.callCount()
	err := f.usecase.Compact(f.ctx, chatID)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Equal(t, before, f.term.callCount(),
		"a provider that cannot compact must have nothing sent to it")
}

// A chat that does not exist must fail before anything is sent anywhere.
func TestCompact_AnUnknownChatSendsNothing(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingDescriptorBody)

	before := f.term.callCount()
	err := f.usecase.Compact(f.ctx, "no-such-chat")

	require.Error(t, err)
	assert.Equal(t, before, f.term.callCount(),
		"an unknown chat must reach no CLI at all")
}

// TestCompact_ResolvesCwdThroughTheAncestorWalkForABubble proves Compact
// (compact.go) resolves a bubble's cwd through the SAME ancestor walk
// spawnPaths uses (Task 22) — a genuinely separate call site that used to
// resolve WorktreeDir from chat.WorkspaceID directly.
//
// A no-gesture refusal and a cwd-resolution failure both surface as
// apperr.ErrNotFound here (see TestCompact_AProviderWithNoGestureIsNotFound
// above), so the error alone cannot tell them apart — the assertion that
// actually proves the fallback ran is on f.ws.lastWorkspaceID: fakeWorkspace
// answers every id identically, including "", which is exactly how the
// original bug went undetected through 21 tasks.
func TestCompact_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")

	err := f.usecase.Compact(f.ctx, bubbleID)

	require.ErrorIs(t, err, apperr.ErrNotFound,
		"must still refuse for the provider's OWN missing gesture, exactly as an ordinary chat would")
	assert.Equal(t, "ws1", f.ws.lastWorkspaceID,
		"must resolve the bubble's cwd through its workspace-owning ancestor, not its own empty WorkspaceID")
}

// ─── from boot_test.go ────────────────────────────────────────────────

// TestRegression_DeadPTY_MeansDeadRunner: a runner cannot outlive its PTY. Boot
// reconciliation is the ONE place liveness is reconciled, and it reconciles against the
// single authority — the PTY — rather than against a second opinion Crowbar stored.
//
// What must NOT go with the runner is the chat: killing the live row is what makes the
// chat dormant, and dormant is the state Resume revives from. A reconcile that took the
// conversation history with it would turn a restart into data loss.
func TestRegression_DeadPTY_MeansDeadRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	f.term.dieWithDaemon() // the daemon restarted; every PTY is gone

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	_, err := f.runners.LiveRunnerForChat(f.ctx, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "no runner may outlive its PTY")

	// ...but the chat is still resumable: its conversation history is append-only and
	// describes what HAPPENED, which no restart can falsify.
	last, err := f.runners.LastConversation(f.ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, "s1", last.SessionID)
}

// TestRegression_AfterRestart_ResumeStillWorks is the headline regression. The live-runner
// table is durable sqlite and is never truncated at boot, so without this reconcile every
// chat that ever had a runner is UNREVIVABLE for the rest of time: ResumeChat asks
// LiveRunnerForChat first, is handed the stale row of a CLI that died with the daemon, and
// returns it as a no-op. The Resume button silently does nothing, and the pane attaches to
// a dead terminal session.
func TestRegression_AfterRestart_ResumeStillWorks(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	turn(t, f, runnerID, "claude", "the reply the user came back for")

	f.term.dieWithDaemon()

	// The row really does survive the death of the process it describes. That is not a
	// bug in the model — the row is only ever removed by an Exit, and an Exit is only ever
	// emitted because the PTY died, which is a fact nothing was alive to observe.
	stale, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err, "precondition: the pre-restart runner row outlives the daemon")
	require.Equal(t, runnerID, stale.ID)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)

	assert.NotEqual(t, runnerID, revived,
		"Resume must spawn a NEW runner, not hand back the id of a CLI that died with the daemon")
	assert.Equal(t, 2, f.term.callCount(), "Resume must actually launch a vendor CLI")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID, "and the chat is held by the CLI the user can actually talk to")
}

// TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever: AgentChat.Working is
// reconciled state, never durable truth — a CLI that dies mid-turn never sends the
// turn_stop hook that would close it. When the daemon dies mid-turn there is nobody left
// to run the runtime exit reconcile either, so the chat comes back Working, spins forever,
// and keeps the whole workspace's overlay spinning with it.
//
// Closing that turn asserts nothing about any process: the runner it belonged to is gone
// (we have just Exited it, on the PTY's authority) and no other runner is on the chat, so
// "nobody is working on this chat" is simply the last true thing we can say about it.
func TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "a long-running task the daemon died in the middle of"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	f.term.dieWithDaemon()

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.False(t, f.chat(t, chatID).Working,
		"a chat that was mid-turn when the daemon died must not spin forever")
}

// TestReconcileRunnersOnBoot_ReapsADisplacedRunnerWhoseKillFailed pins the ONE runner
// nothing else in the system will ever clean up.
//
// Displace takes a runner off its chat while its process is still alive, and the kill that
// follows it is best-effort. When that kill genuinely fails, the runner is left placed
// NOWHERE (empty CurrentChatID), owned by nobody, and never Exited — no hook can reach it,
// no chat points at it, and no teardown path will visit it again. Its row is immortal, and
// the rows accumulate across restarts. Boot reconcile is the only thing that reaps it,
// which is why the reconcile must be driven off ALL live runners rather than off the chats.
func TestReconcileRunnersOnBoot_ReapsADisplacedRunnerWhoseKillFailed(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	require.DirExists(t, tmpDir, "precondition: the spawned CLI has a tmp dir")

	f.term.terminateErr = errors.New("boom: the SIGTERM did not land")

	// A chat delete displaces the runner and then fails to kill it.
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	displaced, err := f.runners.Get(f.ctx, runnerID)
	require.NoError(t, err, "precondition: a displaced runner whose kill failed keeps its live row")
	require.Empty(t, displaced.CurrentChatID, "precondition: Displace erased the chat pointer")

	f.term.dieWithDaemon()

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	_, err = f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound,
		"a runner placed nowhere is reaped by boot reconcile or by nothing at all")

	// And its tmp dir goes too — which is only possible because the path is derived from the
	// runner id and provider. Nothing on this row still names the chat it was spawned into,
	// so a chat-keyed path would be unreachable here.
	assert.NoDirExists(t, tmpDir, "the crash-orphan tmp dir of a displaced runner must be reaped")
}

// TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir: on a clean exit the onExit callback
// removes the runner's tmp dir. A crash is precisely the case where that callback never ran,
// so these dirs are the one orphan class that would otherwise accumulate forever, one per
// spawn, across every restart.
//
// They hold the rendered hook config and nothing else — no credentials (the engine has no
// copy_file verb, and no descriptor references one) — so this is hygiene, not a leak.
func TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	require.DirExists(t, tmpDir, "precondition: the spawned CLI has a tmp dir")

	f.term.dieWithDaemon() // the daemon died: onExit never fired, so the dir was never removed
	require.DirExists(t, tmpDir, "precondition: a crashed daemon reaps nothing on its way out")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.NoDirExists(t, tmpDir, "boot reconcile must reap the tmp dir of a CLI that died with the daemon")
}

// TestReconcileRunnersOnBoot_LeavesALiveRunnerAlone: the reconcile is not a truncation. It
// asks the PTY about every runner and Exits only the ones the PTY says are gone — so a
// runner whose CLI is genuinely still running (the daemon did not restart; something else
// called this) is left exactly where it is, still holding its chat.
func TestReconcileRunnersOnBoot_LeavesALiveRunnerAlone(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err, "a runner whose PTY is alive is not reaped")
	assert.Equal(t, runnerID, live.ID)
}

// TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer: on an idle machine nothing is running,
// and "nothing is running" is a real answer, not a failure.
func TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
}

// TestReconcileRunnersOnBoot_WorkspacelessChatDoesNotBreakTheSweep pins spec §1.5's
// worst failure mode: before this fix, the prompt-journal boot sweep resolved every
// chat's directory through AgentChatsDir(chat.WorkspaceID), which requires a
// resolvable workspace row. A single chat with no workspace (a "bubble" — model spec
// §3.1) would fail that lookup and abort reconciliation for EVERY chat, not just its
// own — one unplaced chat bricking every other chat's boot recovery.
func TestReconcileRunnersOnBoot_WorkspacelessChatDoesNotBreakTheSweep(t *testing.T) {
	f := newFixture(t)

	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx),
		"a workspace-less chat must not fail the boot sweep for every chat")

	_, err = f.usecase.GetChat(f.ctx, bubbleID)
	require.NoError(t, err)
}

// ─── from stop_test.go ────────────────────────────────────────────────

// TestStopChat_TerminatesTheCLI_LeavesChatDormantAndResumable is the headline: closing
// a chat tab STOPS the vendor CLI but must leave the chat exactly where a later reopen
// can revive the REAL conversation. It drives the whole life-cycle a close touches —
// terminate the live runner, drop to dormant, clear a mid-turn spinner, keep the chat
// and its bound conversation — and then proves resumability end-to-end via ResumeChat.
func TestStopChat_TerminatesTheCLI_LeavesChatDormantAndResumable(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	require.NotEmpty(t, oldTerm)

	// A real, resumable conversation: the CLI announced its native session and took a
	// turn (a session id is not a conversation until something is written to it).
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said something")

	// And it is mid-answer when the user closes the tab — the state whose spinner the
	// close must clear.
	prompt(t, f, runnerID, "claude", "and one more thing")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	// (1) the vendor CLI was gracefully terminated.
	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "close must gracefully terminate the live CLI")

	// (2) the chat is DORMANT — no runner points at it any more.
	_, err := f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "after close the chat has no live runner")

	// (3) the chat still EXISTS, and its Working spinner was cleared (the aborted turn).
	stopped := f.chat(t, chatID)
	assert.False(t, stopped.Working, "closing mid-turn must clear the spinner, not leave it spinning forever")

	// (4) its bound conversation is retained — this is what makes it resumable.
	convs, err := f.usecase.ConversationsForChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, convs, 1)
	assert.Equal(t, "sid-claude-native", convs[0].SessionID, "a close must keep the conversation it can be resumed into")

	// The outgoing runner stays alive until its PTY actually dies — Crowbar never asserts
	// a death it has not observed. Then the exit reconcile lands it Exited.
	f.term.exit(t, oldTerm)
	f.wait()
	_, err = f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "the PTY's death carries the runner away (Exited)")

	// (5) reopening revives the REAL conversation: the last provider, resumed into its
	// own native session, exactly where the user left it.
	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "revive brings back the provider that was last here")

	require.Equal(t, 2, f.term.callCount(), "resume spawns a fresh CLI on the same chat")
	assert.Equal(t, "sid-claude-native", argAfter(t, f.term.calls[1].argv, "--resume"),
		"revive must resume the CLI's OWN conversation, not start a blank one")
}

// TestStopChat_AbortsInFlightTurn_DoesNotWait is the user's explicit choice made testable:
// "close = stop immediately". Unlike SwitchProvider, StopChat must NOT wait for the
// in-flight turn — it terminates the CLI mid-answer and clears the spinner right away.
// The proof is a NEGATIVE (the close never parks on the turn), taken at a moment the test
// knows the close has run: a straight-line call that returns.
func TestStopChat_AbortsInFlightTurn_DoesNotWait(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	parked := parkedOnTurn(t)

	// Straight-line: no goroutine. A switch would PARK here (waiting for the turn) and
	// this call would never return; a close aborts the turn and returns immediately.
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	select {
	case <-parked:
		t.Fatal("close = stop immediately: StopChat must never wait for the in-flight turn")
	default:
	}

	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "the mid-turn CLI is terminated — the abort is intended")
	assert.False(t, f.chat(t, chatID).Working, "the aborted turn's spinner is cleared at once")
}

// TestStopChat_AlreadyDormant_IsNilNoop: a chat whose CLI is already gone has nothing to
// stop, so the close is a clean no-op — never an error, and it terminates nothing.
func TestStopChat_AlreadyDormant_IsNilNoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI exits on its own
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID), "stopping an already-dormant chat is a nil no-op")
	f.wait()

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	assert.NotEmpty(t, f.chat(t, chatID).ID, "the chat is still there — a no-op close does not remove it")
}

// TestStopChat_TerminateFailure_StillDropsToDormant proves the ordering that makes a close
// reliable: DISPLACE FIRST, terminate best-effort. Even when the SIGTERM genuinely fails,
// the chat is already dormant (the placement fact was recorded before the kill), and the
// close does not wedge — a failed terminate leaks a process that dies on its own, never a
// close the user asked for.
func TestStopChat_TerminateFailure_StillDropsToDormant(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID), "a best-effort close never fails on a terminate error")
	f.wait()

	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "the terminate was still ATTEMPTED")
	_, err := f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "displace-first means the chat is dormant even when the kill fails")
}

// TestStopChat_DoesNotDeleteTheChat pins the boundary against PurgeChat: a close STOPS the
// process but must never Forget the chat — it stays in the read model, addressable and
// resumable, exactly the difference between closing a tab and deleting a chat.
func TestStopChat_DoesNotDeleteTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	got := f.chat(t, chatID)
	assert.Equal(t, chatID, got.ID, "a close must not delete the chat")

	all, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	var found bool
	for _, c := range all {
		if c.ID == chatID {
			found = true
		}
	}
	assert.True(t, found, "the stopped chat still appears in the chat list")
}

// ─── from agent_test.go ───────────────────────────────────────────────

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
// surfaced as POST /chats 500 and a chat button that did nothing.
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
		"created",              // SpawnChat
		"permission_level_set", // SpawnChat: seeded from the global default
		"title_set",            // user_prompt: derived title
		"turn_started",         // user_prompt: StartTurn
		"turn_stopped",         // turn_stop: StopTurn
		"created",              // /clear: the minted chat
		"permission_level_set", // /clear: seeded from the global default
		"deleted",              // PurgeChat (asynx Forget's OnForget)
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

// TestRegression_SpawnRefusedAfterTheChatWasWritten_ForgetsTheChat pins the rule
// TestRegression_DisabledProviderIsRefusedNotJustHidden already states in words — "a
// refused spawn must not leave a chat behind" — on the two refusals that actually
// broke it.
//
// A provider refused BEFORE anything is written (a disabled one, an unresolvable
// descriptor, a CLI that is not installed) never had a chat to leave, which is why
// that rule looked kept. The chat is written mid-spawn, by recordRunner, and BOTH
// refusals downstream of it returned the error while leaving the chat standing: a CLI
// that exits during startup (424) and a runner row that will not commit (500). The
// caller was told the spawn failed and could then list the chat it had made — the
// record contradicting its own response, the same defect class as a prompt recorded
// `answered` whose bytes never reached the CLI.
//
// The frame sequence is what makes this non-vacuous: it asserts the chat was really
// CREATED before it was deleted, so the test cannot pass by the spawn failing earlier
// than the bug it is about.
func TestRegression_SpawnRefusedAfterTheChatWasWritten_ForgetsTheChat(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(f testFixture, rs *fakeRunnerStore)
	}{
		{
			// The production case: `true` as a vendor CLI — a process that swears it is
			// interactive and is already gone by the time its row could be written.
			name: "the CLI exits during startup",
			arm: func(f testFixture, _ *fakeRunnerStore) {
				f.term.duringForkCall = func(call commandCall) { call.onExit() }
			},
		},
		{
			name: "the runner row will not commit",
			arm: func(_ testFixture, rs *fakeRunnerStore) {
				rs.failStart = fmt.Errorf("boom: start runner: %w", asynxModels.ErrValidation)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, rs := newFaultFixture(t)
			tc.arm(f, rs)

			chatID, runnerID, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
			require.Error(t, err)
			assert.Empty(t, chatID, "a refused create hands back no chat")
			assert.Empty(t, runnerID)
			f.wait()

			assert.Equal(t, []string{"created", "permission_level_set", "deleted"}, f.bcKinds(t),
				"the chat was written and then taken back out — not merely never written")

			var born string
			for _, frame := range f.bc.snapshot() {
				if frame.kind == "created" {
					born = frame.chatID
				}
			}
			require.NotEmpty(t, born, "precondition: the spawn got far enough to write a chat")

			_, getErr := f.chats.GetChat(f.ctx, born)
			assert.ErrorIs(t, getErr, agentchat.ErrNotFound,
				"the aggregate is Forgotten, not just hidden from the list")
			listed, err := f.chats.ListChats(f.ctx)
			require.NoError(t, err)
			assert.Empty(t, listed, "a refused spawn must not leave a chat behind")
		})
	}
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

// TestRegression_TurnStop_AssistantMessageIsDurableBeforeTurnStateIsPublished pins the
// ordering the React chat depends on. StopTurn's projection broadcasts Working=false,
// and the chat view treats that edge as its cue to do ONE ledger read and then stop
// polling. While StopTurn ran BEFORE the ledger append, that read could be served
// before the assistant row existed — and with the turn over and the queue empty,
// nothing ever re-read it. Live on 2026-08-16 that stranded a real reply in the ledger:
// the user's message rendered, the answer did not, and it only appeared after an
// unrelated refresh (switching chats).
//
// The seam fires inside StopTurn, which is the earliest moment any client can learn the
// turn ended. The ledger must already hold the answer by then.
func TestRegression_TurnStop_AssistantMessageIsDurableBeforeTurnStateIsPublished(t *testing.T) {
	f, cs, _ := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "ask"})))
	f.wait()

	var ledgerAtPublish string
	cs.onStopTurn = func() {
		handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
		require.NoError(t, err)
		ledgerAtPublish = handoff
	}

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the answer"})))
	f.wait()

	assert.Contains(t, ledgerAtPublish, "assistant (claude): the answer",
		"the assistant message must be readable BEFORE the turn-state change is published, "+
			"or a client that reads once on that edge and stops polling never sees the reply")
}

// TestRegression_TurnStop_EmptyMessageStillClosesTheTurn guards the invariant the old
// ordering existed to protect: an empty assistant message is a ledger no-op, but must
// never be a turn-state no-op. Appending first must not make the turn depend on there
// being something to append.
func TestRegression_TurnStop_EmptyMessageStillClosesTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "ask"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1"})))
	f.wait()

	chat, err := f.usecase.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.False(t, chat.Working, "an empty turn_stop must still close the turn")
}

func TestListChatsByWorkspace_ReturnsOnlyThatWorkspacesChats(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	mine, err := f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, err)
	ids := make([]string, 0, len(mine))
	for _, c := range mine {
		ids = append(ids, c.ID)
	}
	assert.Contains(t, ids, chatID)

	other, err := f.usecase.ListChatsByWorkspace(f.ctx, "ws-elsewhere")
	require.NoError(t, err)
	assert.Empty(t, other)
}

// A retry after a lost HTTP response must receive the ORIGINAL delivery, even
// though that delivery has since made the chat busy. Reporting "busy" instead
// would tell a client its message failed when it had already been sent.
func TestSubmitPrompt_ARetryAfterAConfirmedDeliveryReturnsTheOriginal(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	const message = "the prompt that landed"
	requestID := uuid.NewString()

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": message, "transcript_path": "/rollouts/x"})))
	f.wait()
	spawnsBefore := f.term.callCount()

	second, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)

	require.NoError(t, err)
	assert.Equal(t, first, second, "a retry reports the delivery that already happened")
	assert.Equal(t, spawnsBefore, f.term.callCount(), "and starts no second CLI")

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, turns, 1, "the prompt is recorded once")
}

// ─── from agent_errors_test.go ────────────────────────────────────────

func TestPromptErrorCode_NamesEveryPromptFailure(t *testing.T) {
	testCases := []struct {
		err  error
		want string
	}{
		{agentusecase.ErrPromptBusy, agentusecase.PromptCodeBusy},
		{agentusecase.ErrPromptOutcomeUnknown, agentusecase.PromptCodeOutcomeUnknown},
		{agentusecase.ErrPromptAlreadyAccepted, agentusecase.PromptCodeAlreadyAccepted},
		{agentusecase.ErrPromptRequestIDConflict, agentusecase.PromptCodeRequestIDConflict},
		{agentusecase.ErrPromptUnsupported, agentusecase.PromptCodeUnsupported},
		{agentusecase.ErrPromptSessionUnavailable, agentusecase.PromptCodeSessionRequired},
	}
	seen := map[string]struct{}{}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, agentusecase.PromptErrorCode(tc.err))

			assert.Equal(t, tc.want,
				agentusecase.PromptErrorCode(fmt.Errorf("agent: submit prompt: %w", tc.err)))
		})
		_, dup := seen[tc.want]
		assert.False(t, dup, "codes must be distinct: %s", tc.want)
		seen[tc.want] = struct{}{}
	}
}

func TestPromptErrorCode_HasNoCodeForAnUnrelatedFailure(t *testing.T) {
	assert.Empty(t, agentusecase.PromptErrorCode(errors.New("disk gone")))
	assert.Empty(t, agentusecase.PromptErrorCode(nil))
}

func TestCatalogErrorCode_NamesEveryCatalogueFailure(t *testing.T) {
	testCases := []struct {
		err  error
		want string
	}{
		{agentusecase.ErrSlashCatalogUnsupported, agentusecase.CatalogCodeUnsupported},
		{agentusecase.ErrSlashCatalogNoLiveTUI, agentusecase.CatalogCodeLiveRequired},
		{agentusecase.ErrSlashCatalogTimeout, agentusecase.CatalogCodeTimeout},
		{agentusecase.ErrSlashCatalogUnavailable, agentusecase.CatalogCodeUnavailable},
		{agentusecase.ErrSlashCatalogOutputLimit, agentusecase.CatalogCodeOutputLimit},
		{agentusecase.ErrSlashCatalogCommand, agentusecase.CatalogCodeCommand},
		{agentusecase.ErrSlashCatalogMalformed, agentusecase.CatalogCodeMalformed},
		{agentusecase.ErrSlashCatalogSuperseded, agentusecase.CatalogCodeSuperseded},
	}
	seen := map[string]struct{}{}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, agentusecase.CatalogErrorCode(tc.err))
			assert.Equal(t, tc.want,
				agentusecase.CatalogErrorCode(fmt.Errorf("agent: slash catalog: %w", tc.err)))
		})
		_, dup := seen[tc.want]
		assert.False(t, dup, "codes must be distinct: %s", tc.want)
		seen[tc.want] = struct{}{}
	}
}

func TestCatalogErrorCode_HasNoCodeForAnUnrelatedFailure(t *testing.T) {
	assert.Empty(t, agentusecase.CatalogErrorCode(errors.New("disk gone")))
	assert.Empty(t, agentusecase.CatalogErrorCode(nil))
}

// ─── from terminal_wait_test.go ───────────────────────────────────────

const trustDialog = "❯ 1. Yes, I trust this folder\n  2. No, exit\n  Enter to confirm · Esc to cancel"

func TestUsecase_MatchTerminalPrompt_ResolvesTheShippedDescriptor(t *testing.T) {
	f := newFixture(t)

	prompt, ok := f.usecase.MatchTerminalPrompt(f.ctx, "claude", trustDialog)

	require.True(t, ok)
	assert.Equal(t, domain.AgentTerminalWaitTrust, prompt.Kind)
}

func TestUsecase_MatchTerminalPrompt_UnknownProviderIsSilent(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalPrompt(f.ctx, "telepathy", trustDialog)

	assert.False(t, ok)
}

func TestUsecase_MatchTerminalPrompt_OrdinaryScreenIsNotABlock(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalPrompt(f.ctx, "claude", "> Ready.\n  shift+tab to cycle")

	assert.False(t, ok)
}

// ─── from message_record_test.go ──────────────────────────────────────

// deltaCallbackRecorder captures every call the usecase makes on the live
// message-delta callback it was handed at sweep start.
type deltaCallbackRecorder struct {
	mu    sync.Mutex
	calls []deltaCall
}

type deltaCall struct {
	chatID      string
	workspaceID string
	messageID   string
	text        string
}

func (r *deltaCallbackRecorder) record(chatID, workspaceID, messageID, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, deltaCall{chatID, workspaceID, messageID, text})
}

func (r *deltaCallbackRecorder) snapshot() []deltaCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]deltaCall(nil), r.calls...)
}

func (r *deltaCallbackRecorder) texts() []string {
	out := []string{}
	for _, c := range r.snapshot() {
		out = append(out, c.text)
	}
	return out
}

// deltaHook renders one claude `message_delta` payload exactly as the bundled
// descriptor declares it (session_id/turn_id/message_id/index/final/delta), so
// the real engine parses it rather than a shape invented here.
func deltaHook(t *testing.T, messageID string, index int, final bool, text string) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"session_id": "sess-1",
		"turn_id":    "turn-1",
		"message_id": messageID,
		"index":      index,
		"final":      final,
		"delta":      text,
	})
}

// TestStartTerminalWaitSweep_PushesEveryDeltaAsTheMessageSoFar pins the live
// streaming callback: the thing that puts an assistant message on screen WHILE
// it is being said. It is a plain field on the usecase, assigned at sweep start
// and read on every delta, so nothing about it fails to compile if it stops
// being called — the chat pane simply goes silent until the turn ends.
//
// It asserts the CUMULATIVE text, not the increment. A client that missed one
// frame must be correct again on the next one with no reassembly of its own,
// which is only true if each call carries everything said so far.
//
// No goroutine and no timing: fakeCommander implements no Screen method, so
// newTerminalWaitDetector finds no termwait.Screens, u.termWait stays nil, and
// StartTerminalWaitSweep assigns the callbacks and returns without starting the
// sweep. Every call below therefore lands on this test's own goroutine, inside
// IngestHookDelivery.
func TestStartTerminalWaitSweep_PushesEveryDeltaAsTheMessageSoFar(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	deltas := &deltaCallbackRecorder{}
	f.usecase.StartTerminalWaitSweep(f.ctx, nil, nil, deltas.record)

	post := func(index int, final bool, text string) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "ws1", uuid.NewString(), runnerID, "claude", "message_delta",
			deltaHook(t, "msg-one", index, final, text),
		))
	}
	post(0, false, "THE ")
	post(1, false, "MESSAGE ")
	post(2, true, "SO FAR")
	f.wait()

	assert.Equal(t,
		[]string{"THE ", "THE MESSAGE ", "THE MESSAGE SO FAR"},
		deltas.texts(),
		"every delta must push the message SO FAR, so a client that dropped a frame is correct on the next one")

	for _, call := range deltas.snapshot() {
		assert.Equal(t, chatID, call.chatID, "the frame must name the chat it belongs to")
		assert.Equal(t, "ws1", call.workspaceID, "the feed is workspace-scoped")
		assert.Equal(t, "msg-one", call.messageID,
			"the provider's own message identity is how a client tells a growing message from the next one")
	}

	// The partials exist ONLY on this callback. Nothing durable ever held "THE "
	// or "THE MESSAGE ", so a build that stopped calling it would leave the pane
	// with nothing to show until the turn ended — and the ledger below would
	// still read exactly the same.
	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "THE MESSAGE SO FAR", page.Items[0].Text)
}
