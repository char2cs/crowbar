package chat_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ─── from switch_test.go ──────────────────────────────────────────────

// A switch's preflight resolves a bubble's cwd through the ancestor walk too.
// The whole call list is asserted: the spawn resolves "ws1" last either way, and
// fakeWorkspace answers "" as readily as a real id.
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

	// The switch returned only once the outgoing CLI's exit was recorded: a
	// replacement never lands beside it.
	assert.False(t, f.isLive(oldRunner))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, ledger.RunnerExit{RunnerID: oldRunner, Reason: domain.AgentExitDisplaced, At: exit.At}, exit)
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

// TestSwitchProvider_Broadcasts_TheChatsOwnProviderChange: a handoff changes which
// CLI is on the chat, and the runner feed carries that whole story in order — the
// outgoing runner exits, and only then does the incoming one start.
//
// The chat aggregate is written to exactly ONCE, and only for what durably moved: its
// own vendor (domain.Chat.ProviderID), the same way SwitchToTerminal writes its
// surface. A respawn on the SAME provider changes nothing and emits nothing — the
// command refuses an unchanged value — so this frame appears only on a real switch.
func TestSwitchProvider_Broadcasts_TheChatsOwnProviderChange(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.bc.reset()
	f.snapFrames.reset()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Equal(t, []string{"provider_set"}, f.bcKinds(t),
		"the only chat-aggregate write a switch makes is the chat's own vendor moving")
	assert.Equal(t, []string{"exited", "started"}, f.runnerKinds(t))
}

// TestSwitchProvider_RecordsAProviderSwitchedInterruption guards the new
// InterruptProviderSwitched marker: recorded once the switch actually
// commits, and only when the target differs from the chat's provider
// beforehand — Crowbar's own doing, not something a provider hook reports.
func TestSwitchProvider_RecordsAProviderSwitchedInterruption(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, agents.InterruptProviderSwitched, ints[0].Kind)
	assert.Equal(t, "codex", ints[0].Detail)
	assert.NotNil(t, ints[0].ResolvedAt, "Crowbar's own doing: opened and resolved together")
}

// TestSwitchProvider_SwitchingToTheSameProvider_RecordsNothing guards the
// no-op case ResumeChat exercises every day: resuming into the provider the
// chat is ALREADY on is not a switch worth marking.
func TestSwitchProvider_SwitchingToTheSameProvider_RecordsNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, ints, "switching to the provider already on the chat is not a switch")
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
	exchange(t, f, runnerID, "claude", "claude said something")

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

// With no api connection (this fixture disables it, as production degrades to
// when app-server fails), the hooks PTY is the only codex: it must carry the
// native `resume {id}` itself, or codex mints a new thread on every switch back.
func TestSwitchProvider_SwitchBack_ResumesItsOwnSessionOverTheOnlyCodexThereIs(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner := f.spawn(t, "codex")
	f.announce(t, codexRunner, "sid-codex-native")
	exchange(t, f, codexRunner, "codex", "codex ledger content")

	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	// What codex misses while it is away.
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0,
		"with no api connection holding the thread, the PTY IS codex and must resume its own session: %v", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-codex-native", argv[resumeIdx+1],
		"the resume subcommand must name codex's own prior thread: %v", argv)
	for _, tok := range argv {
		assert.NotContains(t, tok, "[Crowbar]",
			"codex declares no resume_context_inject: its gap never rides argv as a pointer it would answer "+
				"as an unrelated second conversation: %v", argv)
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
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

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

	// A workspace-reader failure surfaces wrapped, at the preflight WorktreeDir
	// read: the pending-prompt wait before it reads only the chat ledger.
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
	exchange(t, f, runnerID, "claude", "claude said something")

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

// Reviving a chat revives the provider it was on, even one that never bound a
// conversation; reading only an older provider's row switched the chat back
// with no /switch anywhere.
func TestRegression_ResumeChat_RevivesProviderThatNeverBoundAConversation(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	turn(t, f, claudeRunner, "claude", "claude said something")
	// The switch marker's own timestamp must strictly postdate claude's
	// conversation row, or the two land on the same instant and the comparison
	// this test exists to prove has nothing to distinguish.
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	// No f.announce for codex: it never reports a session_start hook, so it
	// never binds a conversation row — the exact shape the live report's own
	// "ruled out" note describes.

	f.term.exit(t, f.runner(t, codexRunner).TerminalSession) // the CLI dies
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "codex", live.ProviderID,
		"resume must bring back codex — the provider actually running when the chat "+
			"went dormant — not claude, whose conversation is merely older")
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
	exchange(t, f, claudeRunner, "claude", "claude ledger content")
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
	codexTurnStop(t, f, codexRunner, "sid-codex-native", "codex spoke while claude was away")

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

// Switch-then-send through SubmitPrompt: the replacement spawn that delivers the
// text must carry the gap the switch assembled, not recompute that none is owed
// because the session had turned before the switch away.
func TestRegression_SubmitPromptWithStagedProvider_DeliverySpawnCarriesTheGap(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude ledger content")
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")

	codexTurnStop(t, f, codexRunner, "sid-codex-native", "codex spoke while claude was away")

	// The combined path: switch back to claude AND deliver a prompt in one
	// call, exactly what the composer does on a staged-provider send.
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	// spawn 0: initial claude. spawn 1: switch to codex. spawn 2: SubmitPrompt's
	// internal SwitchProvider back to claude (the silent resume, displaced before
	// it can answer). spawn 3: SubmitPrompt's own replacement spawn — the one
	// that actually carries "what did I miss?" and is what claude answers from.
	require.Equal(t, 4, f.term.callCount())
	argv := f.term.calls[3].argv

	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"the delivery spawn must resume claude's own conversation, not start a blank one")

	joined := strings.Join(argv, "\x00")
	assert.Contains(t, joined, "what did I miss?",
		"the delivery spawn must carry the user's actual prompt: argv was %v", argv)
	assert.Contains(t, joined, "codex spoke while claude was away",
		"the delivery spawn — not just the switch's own throwaway silent resume — "+
			"must carry the gap claude missed while away: argv was %v", argv)
	assert.NotContains(t, joined, "claude ledger content",
		"a provider resumed into its own conversation must not be re-fed its own earlier turns")

	// Not just present SOMEWHERE in argv — in the SAME trailing positional as
	// the gap, behind exactly one "--". Two SEPARATE positionals (gap, then
	// its own later "-- what did I miss?") is the exact shape that reached
	// claude's real CLI live: it attended to only the first and the second
	// was silently dropped, never even reaching the CLI's own user_prompt
	// hook — see mergeLeadingPositional.
	last := argv[len(argv)-1]
	assert.Contains(t, last, "codex spoke while claude was away")
	assert.Contains(t, last, "what did I miss?")
	assert.Equal(t, 2, len(argv)-indexOf(argv, "--"),
		"exactly one argv entry must follow the final \"--\": argv was %v", argv)
}

// Echo suppression matches by containment, so a spawn carrying a real prompt
// alongside the injected document must not register it: the user's own message
// would be suppressed with it and never reach the ledger.
func TestRegression_SubmitPromptWithStagedProvider_RealPromptIsRecordedNotSuppressed(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude ledger content")
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")
	codexTurnStop(t, f, codexRunner, "sid-codex-native", "codex spoke while claude was away")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	argv := f.term.calls[f.term.callCount()-1].argv
	delivered := argv[len(argv)-1]

	// Exactly what the real claude CLI's own user_prompt hook fires with: the
	// combined positional this spawn was handed, on the session it resumed.
	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": delivered, "session_id": "sid-claude-native"})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "what did I miss?",
		"a spawn carrying a real prompt is never a bare echo — it belongs in the ledger "+
			"like any other user turn, injected preamble and all:\n%s", handoff)
	assert.True(t, f.chat(t, chatID).Working, "the CLI is answering the real prompt: the chat must read as working")
}

// The ledger row for a prompt delivered with injected context holds the user's
// message, not the merged blob: storing the context would fold it into the next
// gap, nesting handoffs without bound.
func TestAudit_SubmitPromptWithStagedProvider_LedgerMustNotPermanentlyStoreTheInjectedContext(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude ledger content")
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")
	codexTurnStop(t, f, codexRunner, "sid-codex-native", "codex spoke while claude was away")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	argv := f.term.calls[f.term.callCount()-1].argv
	delivered := argv[len(argv)-1]

	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": delivered, "session_id": "sid-claude-native"})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	var storedUserText string
	for _, item := range page.Items {
		if item.Role == "user" && item.Text != "" && strings.Contains(item.Text, "what did I miss?") {
			storedUserText = item.Text
		}
	}
	require.NotEmpty(t, storedUserText, "the real prompt's own user turn must exist in the ledger")
	assert.Equal(t, "what did I miss?", storedUserText,
		"the permanently stored user turn must be exactly what the person typed — "+
			"not Crowbar's own injected context riding the same spawn, which must never "+
			"become durable ledger content: got %q", storedUserText)
}

// TestRegression_SubmitPromptWithStagedProviderModelAndEffort_GapStillDelivered
// closes the gap between "switch alone" and "switch with a model/effort pick
// riding the SAME send" — SetChatSelection's own restart-forcing
// (RequirePromptRestart) is a SEPARATE decision from resolvePromptDelivery's
// gap assembly, and this pins that the two compose correctly: the delivery
// spawn must carry BOTH the newly staged model/effort AND the gap, not one at
// the expense of the other.
func TestRegression_SubmitPromptWithStagedProviderModelAndEffort_GapStillDelivered(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude ledger content")
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")
	codexTurnStop(t, f, codexRunner, "sid-codex-native", "codex spoke while claude was away")

	// Switch back to claude, stage a model/effort change, AND deliver a
	// prompt — all in the one call a real staged-everything send makes.
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", &domain.ChatSelection{Model: "opus", Effort: "high"})
	require.NoError(t, err)

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)
	assert.Equal(t, "opus", live.LaunchModel)
	assert.Equal(t, "high", live.LaunchEffort)

	call := f.term.calls[f.term.callCount()-1]
	modelAt := indexOf(call.argv, "--model")
	require.GreaterOrEqual(t, modelAt, 0, "the staged model must still reach argv: %v", call.argv)
	assert.Equal(t, "opus", call.argv[modelAt+1])

	last := call.argv[len(call.argv)-1]
	assert.Contains(t, last, "codex spoke while claude was away",
		"the gap must still be delivered alongside a staged model/effort change: %v", call.argv)
	assert.Contains(t, last, "what did I miss?")
}

// TestRegression_SubmitPromptWithStagedProvider_DormantChatRevivesOntoTheOtherProviderWithTheGap
// covers reopening an old chat and sending straight to a DIFFERENT provider
// than it was last on, in one action — no explicit Resume first. There is no
// live runner to displace at all, so SubmitPromptWithSwitch's own
// current-vs-staged comparison, resumableConversation's age/turn checks, and
// the delivery spawn's gap assembly all run cold, off nothing but durable
// state. A chat with no live runner is still a chat with real history.
func TestRegression_SubmitPromptWithStagedProvider_DormantChatRevivesOntoTheOtherProviderWithTheGap(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude has real history before going dormant")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")
	turn(t, f, codexRunner, "codex", "codex spoke, then the chat went dormant")

	f.term.exit(t, f.runner(t, codexRunner).TerminalSession)
	f.wait()
	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, "sid-claude-native", argAfter(t, call.argv, "--resume"),
		"reviving straight onto the other provider must still resume its OWN conversation: %v", call.argv)
	last := call.argv[len(call.argv)-1]
	assert.Contains(t, last, "codex spoke, then the chat went dormant",
		"the whole time claude was away — including the gap since before it went dormant — must reach it: %v", call.argv)
	assert.Contains(t, last, "what did I miss?")
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

// TestResumeChat_NoConversationButAProviderOfItsOwn_RevivesThatProvider replaces
// the old TestResumeChat_NoConversation_ReturnsError, whose assertion was
//
//	require.Error(t, err)
//	assert.Contains(t, err.Error(), "no conversation to resume")
//
// on this exact setup. That refusal was the premise behind the live
// data-integrity bug: a chat that never announced a conversation WAS refused,
// and the pane answered the refusal by starting the first enabled provider on
// the chat — converting it. A chat's provider does not depend on its CLI having
// got as far as a session hook, and the chat has recorded its own vendor since
// the moment a runner was placed on it.
//
// The genuine "nothing to resume" case is unchanged and is pinned by
// TestResumeChat_NothingHasEverRunHere_ReturnsError below.
func TestResumeChat_NoConversationButAProviderOfItsOwn_RevivesThatProvider(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
	assert.Equal(t, -1, indexOf(f.term.calls[1].argv, "resume"),
		"and it comes back FRESH: there is still no conversation to resume into")
}

// TestResumeChat_NothingHasEverRunHere_ReturnsError keeps the refusal reachable
// for the one chat that genuinely has nothing to resume as: a bare row no runner
// was ever placed on — a reducer mint, or a row from before the chat carried its
// own vendor at all.
//
// It must refuse with ErrChatProviderUnknown, NOT with ErrNoLiveRunner.
// A dormant chat having no live runner is the normal state and the entire reason
// Resume was called, so reporting a missing RUNNER named the one thing that was
// never in question — what could not be resolved is the PROVIDER.
func TestResumeChat_NothingHasEverRunHere_ReturnsError(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.Error(t, err)
	assert.ErrorIs(t, err, agentusecase.ErrChatProviderUnknown)
	assert.Contains(t, err.Error(), "no longer records which provider it ran")
	assert.ErrorIs(t, err, apperr.ErrNotFound,
		"and it still maps to the 404 the old sentinel did, never a 500")
	assert.NotErrorIs(t, err, agentusecase.ErrNoLiveRunner,
		"a missing RUNNER is the expected dormant state, and is not what failed")
}

// TestRegression_SubmitPromptWithStagedProvider_CodexTargetUnaffectedByMerge is
// a defensive guard for mergeLeadingPositional: codex declares NO
// resume_context_inject (its gap rides the api connection instead, never
// argv), so ContextSteps returns nothing on resume and the merge must never
// fire for it — the replacement PTY's argv must carry exactly the message,
// nothing folded in ahead of it, exactly as it always has.
func TestRegression_SubmitPromptWithStagedProvider_CodexTargetUnaffectedByMerge(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude has real, table-recorded history")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	codexRunner, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	f.announce(t, codexRunner.ID, "sid-codex-native")
	exchange(t, f, codexRunner.ID, "codex", "codex has its own real history now too")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "back to codex now", uuid.NewString(), "codex", nil)
	require.NoError(t, err)

	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, "back to codex now", call.argv[len(call.argv)-1],
		"codex's replacement PTY must carry exactly the message, untouched by the "+
			"positional-merge fix that exists only for claude's own shape: %v", call.argv)
	// The resume subcommand is still there — this fixture has no api connection
	// to take the resume over (see apiResumes), so the PTY carries it. What must
	// NOT appear is a folded-in context document ahead of the message.
	assert.Contains(t, call.argv, "resume",
		"with no api connection, the PTY must still resume codex's own thread: %v", call.argv)
}

// TestSwitchProvider_SwitchBackToProviderWithNoTurns_AssignsANewSession: a provider
// that never spoke here has a session its CLI never wrote, so a switch back assigns
// a new one and hands over the whole conversation.
func TestSwitchProvider_SwitchBackToProviderWithNoTurns_AssignsANewSession(t *testing.T) {
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
	assigned := argAfter(t, argv, "--session-id")
	assert.NotEqual(t, "sid-claude-empty", assigned, "an id its CLI never wrote is never handed out again")
	assert.NotContains(t, argv, "--resume", "a session its CLI never wrote cannot be resumed")
	assert.Contains(t, strings.Join(argv, "\x00"), "codex actually said something")
	assert.Equal(t, assigned, f.conversation(t, chatID, "claude").SessionID)
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
	exchange(t, f, codexRunner, "codex", "codex said something")

	for _, call := range f.term.calls {
		for _, kv := range call.env {
			assert.False(t, strings.HasPrefix(kv, "CODEX_HOME=") && kv != "CODEX_HOME="+os.Getenv("CODEX_HOME"),
				"Crowbar must never own codex's home — its sessions live there")
		}
	}

	// Leave codex and come back: it resumes its own conversation, and Crowbar had no
	// session store to lose in between. This fixture has no api connection
	// (CROWBAR_DISABLE_API_TRANSPORT), so the PTY is the only codex there is and
	// carries the resume itself — see apiResumes (resume_injection.go). Either
	// way it is codex's OWN session id, not one this test could have papered over
	// by owning CODEX_HOME.
	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "codex must resume its own conversation: %v", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-codex", argv[resumeIdx+1],
		"the resumed thread must be the one codex announced before it was switched away: %v", argv)
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

// TestSwitchProvider_MidTurn_NeverCompletesOrDies_IsForcedAfterTheGracePeriod is the
// bug the user hit live: they started a chat with one provider and switched to
// another while the first was still mid-turn, and the switch (and the frontend's
// "Starting <provider>…" spinner) hung forever. WaitsForTheTurnBefore... and
// OutgoingCLIDies... above prove the wait releases on the turn's own two real
// endings (it finishes, or the CLI dies) — this proves it also releases when
// NEITHER ever happens: no turn_stop, no exit, nothing, ever. That gap is real
// precisely because AwaitTurnComplete's own reasoning ("a live CLI finishes or
// dies") never accounted for a CLI that is alive but simply stuck.
func TestSwitchProvider_MidTurn_NeverCompletesOrDies_IsForcedAfterTheGracePeriod(t *testing.T) {
	f := newFixture(t).withSwitchGrace(t, 10*time.Millisecond)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
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
	case r := <-done:
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	// The outgoing CLI never sends turn_stop and never dies. Before the fix this
	// blocked here forever; the grace period above is real, but short, so this
	// still proves the release with no sleep or poll of its own.
	sess := <-killed
	assert.Equal(t, oldTerm, sess, "the stuck outgoing turn is forced the same way an explicit Stop would be")

	got := <-done
	require.NoError(t, got.err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, live.ID)
	assert.Equal(t, "codex", live.ProviderID, "the switch completes once the stuck turn is forced")
	assert.NotEqual(t, runnerID, live.ID)
}

// TestSwitchProvider_MidTurn_ContextCancelled_BeforeTheGracePeriod_StillAbortsCleanly
// guards the two new failure paths against colliding: a caller whose OWN context is
// cancelled while the grace period is also armed must still abort with nothing
// changed, exactly as TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged
// already requires with no grace period configured at all.
func TestSwitchProvider_MidTurn_ContextCancelled_BeforeTheGracePeriod_StillAbortsCleanly(t *testing.T) {
	f := newFixture(t).withSwitchGrace(t, time.Hour)

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
	assert.Empty(t, f.term.terminatedIDs(), "the caller's own cancellation aborts the switch untouched, not the grace period")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID, "the outgoing CLI is still on its chat")
	assert.True(t, f.chat(t, chatID).Working, "and still mid-turn")
}

// TestSwitchProvider_RepeatedlyStuck_AcrossTwoProviders_KeepsForcingCleanly stresses the
// exact shape the user asked for: a single chat hopping between providers, more than
// once, where EACH outgoing CLI in turn is stuck (never completes, never dies). It
// proves forceOutgoingTurn leaves the chat in a state a SECOND switch can force again
// cleanly — not just once — and that each force targets the CORRECT runner's terminal
// session, never a stale one left over from the previous round.
func TestSwitchProvider_RepeatedlyStuck_AcrossTwoProviders_KeepsForcingCleanly(t *testing.T) {
	f := newFixture(t).withSwitchGrace(t, 10*time.Millisecond)

	chatID, claudeRunner := f.spawn(t, "claude")
	claudeTerm := f.runner(t, claudeRunner).TerminalSession
	prompt(t, f, claudeRunner, "claude", "round one: think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: mid-turn on claude")

	killed := terminateSignal(f)
	parked := parkedOnTurn(t)
	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()
	select {
	case <-parked:
	case r := <-done:
		t.Fatalf("round one: switch returned without ever reaching the outgoing CLI: %+v", r)
	}
	sess := <-killed
	assert.Equal(t, claudeTerm, sess, "round one forces claude's own terminal, not a stale one")
	got := <-done
	require.NoError(t, got.err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.Equal(t, "codex", live.ProviderID, "round one lands on codex")
	codexRunner := live.ID
	codexTerm := f.runner(t, codexRunner).TerminalSession
	require.NotEqual(t, claudeTerm, codexTerm)

	// Round two: codex, the chat's NEW outgoing provider, gets stuck exactly the same
	// way claude did. Nothing about having already forced one switch may leave the
	// chat unable to force a second.
	prompt(t, f, codexRunner, "codex", "round two: think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: mid-turn on codex")

	killed = terminateSignal(f)
	parked = parkedOnTurn(t)
	done = make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "claude")
		done <- switchResult{runnerID: id, err: err}
	}()
	select {
	case <-parked:
	case r := <-done:
		t.Fatalf("round two: switch returned without ever reaching the outgoing CLI: %+v", r)
	}
	sess = <-killed
	assert.Equal(t, codexTerm, sess, "round two forces codex's own terminal, not claude's stale one")
	got = <-done
	require.NoError(t, got.err)
	f.wait()

	live, err = f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "round two lands back on a fresh claude")
	assert.NotEqual(t, claudeRunner, live.ID, "a fresh runner, not the one already forced out in round one")
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
	payload := map[string]any{"prompt": message}
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, "user_prompt", mustJSON(t, payload)))
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

// A /clear mid-turn moves the chat onto a new session; nothing else will close
// the old session's turn, so the change does, or Working stays true and the
// workspace spins forever.
func TestRegression_ClearMidTurn_ClosesTheOldSessionsTurn(t *testing.T) {
	f := newFixture(t)

	left, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	// The user typed, and then hit /clear before the answer came back.
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, left).Working, "precondition: the chat is mid-turn")

	f.announce(t, runnerID, "sid-after-clear") // /clear: the chat moves onto a new session
	f.wait()

	require.Equal(t, left, f.runner(t, runnerID).CurrentChatID, "precondition: a /clear keeps the chat")
	assert.False(t, f.chat(t, left).Working,
		"the turn the old session left open must close: its turn_stop names a session the "+
			"chat is no longer on, so nothing else will ever close it")
	assert.Nil(t, f.chat(t, left).CurrentTurnStarted, "a closed turn must clear CurrentTurnStarted")
}

// ─── from race_test.go ────────────────────────────────────────────────

// A CLI is live before its runner can be recorded, so a record that fails tears
// the CLI down unconditionally and still surfaces the error: a CLI nothing
// points at is invisible.
func TestSwitchProvider_ARunnerThatCannotBeRecordedIsTornDown(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	f.refuseRunnerStarted(t)

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.ErrorContains(t, err, "record runner")

	// term-2 is the codex CLI whose runner could not be recorded.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-2",
		"the just-spawned CLI whose runner could not be recorded must be torn down")
	_, err = f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "and the chat is free for the next one")
}

// ─── from spawn_startup_exit_test.go ──────────────────────────────────

func exitDuringFork(f testFixture) {
	f.term.duringForkCall = func(c commandCall) { c.onExit(engineterminal.Exit{}) }
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

func TestSlashCatalogRejectsResultWhenTheProviderChangesDuringProbe(t *testing.T) {
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
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(descriptor)), 0o600,
	))

	chatID, _ := f.spawn(t, "codex")
	probeDone := make(chan error, 1)
	go func() {
		_, err := f.usecase.SlashCatalog(f.ctx, chatID)
		probeDone <- err
	}()

	inFlight, err := os.Open(started)
	require.NoError(t, err, "the deterministic provider command must be in flight")
	require.NoError(t, inFlight.Close())

	_, switchErr := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, switchErr)
	f.wait()
	releaser, err := os.OpenFile(release, os.O_WRONLY, 0)
	require.NoError(t, err)
	require.NoError(t, releaser.Close())

	select {
	case err := <-probeDone:
		require.ErrorIs(t, err, agentusecase.ErrSlashCatalogSuperseded,
			"a catalogue of a provider the chat has left must never be returned")
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
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(`
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
`)), 0o600,
	))

	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogUnsupported)
}

// The probe is its own short-lived command, not a question put to the TUI: a
// dormant chat's catalogue is its provider's, the same as a live one's.
func TestSlashCatalog_ServesADormantChatFromItsOwnProvider(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
	cli := filepath.Join(t.TempDir(), "cli")
	require.NoError(t, os.WriteFile(cli, []byte("#!/bin/sh\ncat <<'J'\n"+
		`[{"content":[{"text":"<skills>\n- review\n</skills>"}]}]`+"\nJ\n"), 0o700))
	writeCatalogDescriptor(t, f, strconv.Quote(cli))
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	catalog, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.NoError(t, err)
	assert.Equal(t, "codex", catalog.ProviderID)
	require.Len(t, catalog.Items, 1)
	assert.Equal(t, "$review ", catalog.Items[0].InsertText)
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
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(fmt.Sprintf(`
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
`, command))), 0o600,
	))
}

// ─── from prompts_test.go ─────────────────────────────────────────────

func TestSubmitPrompt_RejectsNULBeforeJournalOrTUITeardown(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "invalid\x00argv", uuid.NewString(), "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Equal(t, spawnCount, f.term.callCount(), "invalid input must not start a replacement")
	assert.Len(t, f.term.terminatedIDs(), terminatedCount, "invalid input must not touch the outgoing TUI")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
}

func TestSubmitPrompt_FreshLazyCodexNeedsNoBoundSession(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	message := "FIRST REACT MESSAGE"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
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
	exchange(t, f, runnerID, "codex", "the current conversation exists")
	message := "CONTINUE FROM REACT"

	// codex is api-transport, non-hotswap — but this fixture has no api
	// connection to take the resume over (apiResumes, resume_injection.go), so
	// the PTY is the conversation and its argv must ORDER the resume subcommand,
	// the session id and only then the message: `resume {id} -- <message>`.
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeIdx := indexOf(call.argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v", call.argv)
	require.Less(t, resumeIdx+1, len(call.argv))
	assert.Equal(t, "native-session", call.argv[resumeIdx+1])
	assert.Less(t, resumeIdx, len(call.argv)-1, "the resume subcommand must precede the message")
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_FreshClaudeTerminatesVariadicMCPBeforeFinalPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	message := "CLAUDE REACT MESSAGE"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
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

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", uuid.NewString(), "", nil)
	require.NoError(t, err)
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "two", uuid.NewString(), "", nil)
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
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()
	f.term.duringTerminate = nil

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"the old runner's matching hook must not clear the replacement's pending-delivery barrier")
}

func TestSubmitPrompt_RunnerPersistFailureAfterPTYStartIsOutcomeUnknown(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	f.refuseRunnerStarted(t)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", uuid.NewString(), "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	assert.Equal(t, 2, f.term.callCount(), "the replacement PTY started before runner persistence failed")
	_, err = f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "the torn-down replacement leaves the chat free")
}

func TestSubmitPrompt_CompletedStoppedResumedChatKeepsNativeResumeIdentity(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "durable-session")
	exchange(t, f, runnerID, "codex", "completed before the TUI stopped")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, "term-1")
	f.wait()
	resumedID, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "durable-session", f.conversation(t, chatID, "codex").SessionID)
	f.announce(t, resumedID, "durable-session")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "continue after reopen", uuid.NewString(), "", nil)
	require.NoError(t, err)
	// No api connection in this fixture, so the restart's PTY is the conversation
	// and must resume durable-session itself (apiResumes, resume_injection.go).
	argv := f.term.calls[f.term.callCount()-1].argv
	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "durable-session", argv[resumeIdx+1])
}

func TestSubmitPrompt_NativeTUIResumeOfKnownSessionKeepsContext(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "known-session")
	exchange(t, f, runnerID, "codex", "completed in the known conversation")

	f.announce(t, runnerID, "temporary-new-session")
	f.announce(t, runnerID, "known-session")
	require.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
	require.Equal(t, "known-session", f.conversation(t, chatID, "codex").SessionID)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "continue immediately after native resume", uuid.NewString(), "", nil)
	require.NoError(t, err)
	// No api connection in this fixture, so the restart's PTY is the conversation
	// and must resume known-session itself (apiResumes, resume_injection.go) —
	// never temporary-new-session, the one it announced in between.
	argv := f.term.calls[f.term.callCount()-1].argv
	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "known-session", argv[resumeIdx+1])
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
	// codexTurnStop delivers codex's real api-shaped turn_stop (threadId/
	// turn.items[type=agentMessage].text — see codex.yaml), NOT claude's flat
	// last_assistant_message shape that turn() builds — using turn() here
	// would silently extract an empty message.
	codexTurnStop(t, f, codexRunnerID, "codex-session", "I like turtles")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	claudeRunner, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	// The switch's own placeholder is silent — claude reports its OWN session
	// id the moment it starts, well before the user has said anything to it.
	f.announce(t, claudeRunner.ID, "claude-session")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I say before?", uuid.NewString(), "", nil)
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
	exchange(t, f, runnerID, "claude", "first answer")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "a normal follow-up", uuid.NewString(), "", nil)
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
		early, ok := f.usecase.own.RunnerByID(earlyRunnerID)
		require.True(t, ok, "a spawn's hooks resolve from the first byte")
		st, err := f.ledger.State(f.ctx, early.CurrentChatID)
		require.NoError(t, err)
		_, recorded := st.LiveRunner()
		require.False(t, recorded, "precondition: the fork callback runs before runner_started commits")
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

// A switch parked on a prompt another window queued destroys nothing while it
// waits, and the end of its own request ends the wait.
func TestSwitchProvider_DoesNotKillPromptAwaitingAcceptanceFromAnotherWindow(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "queued elsewhere", uuid.NewString(), "", nil)
	require.NoError(t, err)
	spawnCount := f.term.callCount()
	terminated := len(f.term.terminatedIDs())
	parked := parkedOn(t, agentusecase.WaitingForPendingPromptLog)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	go func() {
		select {
		case <-parked:
		case <-ctx.Done():
		}
		cancel()
	}()

	_, err = f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, spawnCount, f.term.callCount())
	assert.Len(t, f.term.terminatedIDs(), terminated,
		"the replacement TUI awaiting its user_prompt hook must remain alive")
}

// segmentIDFromCommand is the runner id a CLI's hooks are addressed to: the
// runner-keyed hook URL its argv carries.
func segmentIDFromCommand(t *testing.T, argv []string) string {
	t.Helper()
	m := regexp.MustCompile(`/chats/runners/([^/"]+)/hooks/`).FindStringSubmatch(strings.Join(argv, "\n"))
	require.NotNil(t, m, "rendered hook URL must carry the runner id")
	return m[1]
}

func TestSubmitPrompt_IdempotentRetryReturnsOriginalSpawnWhilePending(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID, "", nil)
	require.NoError(t, err)
	retry, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID, "", nil)
	require.NoError(t, err)
	assert.Equal(t, first, retry)
	assert.Equal(t, 2, f.term.callCount(), "the retry must not spawn a third provider TUI")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "different operation", requestID, "", nil)
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
			d, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID, "", nil)
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

			_, err := f.usecase.SubmitPrompt(f.ctx, chatID, tc.text, tc.request, "", nil)

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

	_, err := f.usecase.SubmitPrompt(f.ctx, uuid.NewString(), "hello", uuid.NewString(), "", nil)

	require.Error(t, err)
}

// Send is the only intent a client needs: a dormant chat is revived by the
// daemon, through the resume ladder, in the same hold as the delivery.
func TestSubmitPrompt_RevivesADormantChatAndDelivers(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-dormant")
	exchange(t, f, runnerID, "claude", "said before stopping")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello again", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, result.RunnerID, live.ID)
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "sid-dormant", argAfter(t, argv, "--resume"), "revived into its own conversation")
	assert.Contains(t, strings.Join(argv, "\x00"), "hello again")
}

// A retry of a request whose outcome is recorded answers from the ledger —
// it never revives, so a retried send cannot spawn a CLI of its own.
func TestSubmitPrompt_ARetryNeverRevives(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	requestID := uuid.NewString()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "once", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()
	spawned := f.term.callCount()

	_, _ = f.usecase.SubmitPrompt(f.ctx, chatID, "once", requestID, "", nil)

	assert.Equal(t, spawned, f.term.callCount())
}

func TestSubmitPrompt_RefusesAProviderWithNoDeclaredDelivery(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(`
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
`)), 0o600,
	))
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString(), "", nil)

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

			result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
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

// Compact resolves a bubble's cwd through the ancestor walk. The error alone
// cannot show it (a refusal is also ErrNotFound), so the resolved workspace id
// is asserted.
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

// The same provider as compactingDescriptorBody, with compact_pre/compact_post
// also mapped — mirroring claude.yaml, which maps them to
// PreCompact/PostCompact with the same {session_id, trigger} fields.
const compactingWithHooksDescriptorBody = `
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
  compact_pre:
    in: PreCompact
    map:
      session_id: session_id
      trigger: trigger
  compact_post:
    in: PostCompact
    map:
      session_id: session_id
      trigger: trigger
runtime:
  transport: hooks
  hooks:
    format: json
`

// TestRegression_CompactPreSettlesThePendingDeliveryImmediately: /compact is an
// ordinary prompt no user_prompt hook acknowledges, and compact_post often never
// arrives, so compact_pre settles it as consumed by the provider's builtin.
func TestRegression_CompactPreSettlesThePendingDeliveryImmediately(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingWithHooksDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: compact is delivered as an ordinary prompt, with nothing yet confirming it")

	// SubmitPrompt's restart_tui strategy REPLACES the runner to deliver
	// "/compact" — the delivery's own RunnerID (and the compact hooks that
	// follow) belong to that replacement, not the one f.spawn returned.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "compact_pre",
		mustJSON(t, map[string]any{"session_id": "native-session", "trigger": "manual"})))

	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"compact_pre alone must settle the delivery, not leave it for compact_post")
	prompts := crowbarPrompts(t, f, chatID)
	require.Len(t, prompts, 1)
	assert.True(t, prompts[0].Consumed, "the CLI ran its own builtin")
	assert.Equal(t, ledger.ReasonProviderBuiltin, prompts[0].Reason)
}

// TestRegression_CompactPostAlsoSettlesADeliveryCompactPreDidNot is the
// defensive twin: if compaction ever happens with a turn genuinely open (so
// compact_pre's Interrupt call does NOT resolve instantly), compact_post must
// still settle the delivery when it does arrive.
func TestRegression_CompactPostAlsoSettlesADeliveryCompactPreDidNot(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingWithHooksDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	// No compact_pre delivered at all here — standing in for the "not idle"
	// case (a turn open when compaction starts), which this test does not
	// need to construct in full: it only needs SOMETHING left pending when
	// compact_post arrives.
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID))

	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "compact_post",
		mustJSON(t, map[string]any{"session_id": "native-session", "trigger": "manual"})))

	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"compact_post must settle a delivery compact_pre left pending")
}

// ─── from boot_test.go ────────────────────────────────────────────────

// No carrier from before a restart can acknowledge a prompt, so boot settles
// every pending one rather than leave its chat busy.
func TestReconcileRunnersOnBoot_SettlesEveryPendingPromptAsDaemonRestart(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	requestID := uuid.NewString()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "lost to the restart", requestID, "", nil)
	require.NoError(t, err)
	f.restart(t)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

	got := receipt(t, f, chatID, requestID)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonDaemonRestart, got.Reason)
}

// TestRegression_DeadPTY_MeansDeadRunner: no CLI outlives the daemon that
// started it, so boot ends every runner the ledger still holds live — and
// leaves the chat, whose ledger is what Resume revives from.
func TestRegression_DeadPTY_MeansDeadRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	f.restart(t)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "no runner may outlive its PTY")
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, runnerID, exit.RunnerID)
	assert.Equal(t, domain.AgentExitDaemonRestart, exit.Reason)

	// ...but the chat is still resumable: its ledger describes what HAPPENED, which no
	// restart can falsify.
	require.Equal(t, "s1", f.conversation(t, chatID, "claude").SessionID)
}

// TestRegression_AfterRestart_ResumeStillWorks: a chat whose CLI died with the
// daemon comes back as a NEW runner, never as the id of the dead one.
func TestRegression_AfterRestart_ResumeStillWorks(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	turn(t, f, runnerID, "claude", "the reply the user came back for")

	f.restart(t)

	// The ledger still holds the runner live: its exit happened in a process
	// that was gone before it could record it.
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	stale, ok := st.LiveRunner()
	require.True(t, ok, "precondition: the pre-restart runner outlives the daemon in the ledger")
	require.Equal(t, runnerID, stale.RunnerID)

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

// TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever: a CLI that dies
// mid-turn never sends its turn_stop, and after a restart no exit callback is
// left to close it; boot does, since its runner is gone.
func TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "a long-running task the daemon died in the middle of"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	f.restart(t)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.False(t, f.chat(t, chatID).Working,
		"a chat that was mid-turn when the daemon died must not spin forever")
}

// TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir: on a clean exit the onExit callback
// removes the runner's tmp dir. A crash is precisely the case where that callback never ran,
// so these dirs are the one orphan class that would otherwise accumulate forever.
func TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	require.DirExists(t, tmpDir, "precondition: the spawned CLI has a tmp dir")

	f.restart(t) // the daemon died: onExit never fired, so the dir was never removed
	require.DirExists(t, tmpDir, "precondition: a crashed daemon reaps nothing on its way out")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.NoDirExists(t, tmpDir, "boot reconcile must reap the tmp dir of a CLI that died with the daemon")
}

// TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer: on an idle machine nothing is running,
// and "nothing is running" is a real answer, not a failure.
func TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
}

// A chat with no workspace (a bubble, model spec §3.1) must not abort the boot
// sweep for every other chat (spec §1.5).
func TestReconcileRunnersOnBoot_WorkspacelessChatDoesNotBreakTheSweep(t *testing.T) {
	f := newFixture(t)

	bubbleID, err := f.usecase.MintChat(f.ctx, "", "", "")
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
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "after close the chat has no live runner")

	// (3) the chat still EXISTS, and its Working spinner was cleared (the aborted turn).
	stopped := f.chat(t, chatID)
	assert.False(t, stopped.Working, "closing mid-turn must clear the spinner, not leave it spinning forever")

	// (4) its bound conversation is retained — this is what makes it resumable.
	assert.Equal(t, "sid-claude-native", f.conversation(t, chatID, "claude").SessionID,
		"a close must keep the conversation it can be resumed into")

	// The close returned once the CLI's exit was recorded, as a stop.
	assert.False(t, f.isLive(runnerID))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, domain.AgentExitStopped, exit.Reason)

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
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID), "stopping an already-dormant chat is a nil no-op")
	f.wait()

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	assert.NotEmpty(t, f.chat(t, chatID).ID, "the chat is still there — a no-op close does not remove it")
}

// TestStopChat_ATerminateThatFailsKeepsTheRunner: a CLI the terminate could not
// end is still running, so the chat keeps it — live, not stuck mid-exit, so a
// resume answers with it and a later stop can still end it.
func TestStopChat_ATerminateThatFailsKeepsTheRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.ErrorContains(t, f.usecase.StopChat(f.ctx, chatID), "terminate outgoing terminal")

	bounded, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	resumed, err := f.usecase.ResumeChat(bounded, chatID)
	require.NoError(t, err, "a runner left exiting would park the resume")
	assert.Equal(t, runnerID, resumed, "the chat still belongs to the CLI that would not die")

	f.term.terminateErr = nil
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, domain.AgentExitStopped, exit.Reason)
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

// TestRegression_ClearKeepsTheChatAndThePTY: /clear (the CLI announces a session the
// chat never had) moves the chat onto that session and marks the change — same chat,
// same runner, same PTY.
func TestRegression_ClearKeepsTheChatAndThePTY(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "s1")
	exchange(t, f, r1, "codex", "said on s1")
	before := f.runner(t, r1)

	f.announce(t, r1, "s2") // /clear: a conversation nobody has seen

	after := f.runner(t, r1)
	assert.Equal(t, chatA, after.CurrentChatID, "a /clear never takes the CLI off its chat")
	assert.Equal(t, before.TerminalSession, after.TerminalSession, "the PTY must not change on a /clear")
	assert.Equal(t, 1, f.term.callCount(), "no new CLI may be spawned for a /clear")
	assert.Empty(t, f.term.terminateRequestIDs(), "and nothing may be terminated")

	assert.Equal(t, "s2", f.conversation(t, chatA, "codex").SessionID)
	assert.Equal(t, []string{"session_changed:s2"}, f.markers(t, chatA))
	assert.Equal(t, chatA, f.chatForSession(t, "codex", "s1"), "the session it left stays the chat's own")
}

// TestRegression_ResumeOfAnotherChatsSession_IsAConflictNotAMove: a CLI that /resumes a
// session another chat owns stays where it is; its chat keeps its binding and says so.
func TestRegression_ResumeOfAnotherChatsSession_IsAConflictNotAMove(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "sA")
	chatB, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "sB")

	f.announce(t, r1, "sB")

	assert.Equal(t, chatA, f.runner(t, r1).CurrentChatID, "the CLI is never moved onto another chat")
	assert.Equal(t, "sA", f.conversation(t, chatA, "codex").SessionID, "and its chat keeps its own session")
	assert.Equal(t, []string{"session_conflict:sB"}, f.markers(t, chatA))
	assert.Equal(t, chatB, f.chatForSession(t, "codex", "sB"))
	assert.Equal(t, r2, mustLive(t, f, chatB).ID, "nobody is evicted")
	assert.Empty(t, f.term.terminateRequestIDs())
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
	assigned := f.conversation(t, chatID, "claude").SessionID
	assert.NotEmpty(t, assigned, "claude is launched on a session id Crowbar minted for the chat")

	// The chat is live because a runner points at it — not because of a flag on it.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	assert.Equal(t, chatID, call.chatID,
		"the CLI's PTY session is owned by the chat that spawned it")
	assert.Equal(t, f.ws.worktree, call.cwd)
	assert.Equal(t, "claude", filepath.Base(call.argv[0]))
	assert.Equal(t, assigned, argAfter(t, call.argv, "--session-id"))
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

func TestSpawnRunner_CrowbarBinaryPathFallsBackToHomeBinCrowbar(t *testing.T) {
	f := newFixture(t)
	// Override the fixture's default CROWBAR_HOOK_BIN so the path falls back to
	// <home>/bin/crowbar.
	t.Setenv("CROWBAR_HOOK_BIN", "")
	home := f.ws.home

	f.spawn(t, "claude")

	require.Equal(t, 1, f.term.callCount())
	mcpConfig := argAfter(t, f.term.calls[0].argv, "--mcp-config")
	assert.Contains(t, mcpConfig, filepath.Join(home, "bin", "crowbar"),
		"the tool bridge still runs the binary installed under the home")
	data, err := os.ReadFile(argAfter(t, f.term.calls[0].argv, "--settings"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "crowbar hook", "hooks no longer exec the binary per event")
}

// TestSpawn_HookConfigCarriesRunnerAndProvider guards the arg-based spawn attribution:
// the RUNNER id (the crowbarSegmentID every hook carries) and the provider are
// rendered into the hook URLs of the hook config, not injected as an env var.
func TestSpawn_HookConfigCarriesRunnerAndProvider(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	settingsPath := argAfter(t, call.argv, "--settings")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "/chats/runners/"+runnerID+"/hooks/claude/")

	for _, kv := range call.env {
		assert.False(t, strings.HasPrefix(kv, "CROWBAR_SEGMENT_ID="), "env must not carry CROWBAR_SEGMENT_ID: %q", kv)
	}
}

// ---------------------------------------------------------------------------
// session_start — a bind, a change, a conflict, or nothing
// ---------------------------------------------------------------------------

func TestIngestHook_SessionStart_Bind_RecordsTheConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.bc.reset()
	f.snapFrames.reset()

	f.announce(t, runnerID, "sid-abc")

	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID, "a bind stays put: the runner does not move")
	c := f.conversation(t, chatID, "codex")
	assert.Equal(t, "sid-abc", c.SessionID)
	assert.False(t, c.BoundAt.IsZero(), "the conversation is stamped with when it opened")

	// And the session is now OWNED: a later announcement of it elsewhere is a conflict.
	assert.Equal(t, chatID, f.chatForSession(t, "codex", "sid-abc"))
	assert.Empty(t, f.markers(t, chatID), "binding an unbound chat is not a change")

	assert.Empty(t, f.runnerKinds(t), "the session is the chat's, never the runner's")
	assert.Empty(t, f.bcKinds(t), "binding a conversation writes nothing to the chat")
}

// TestIngestHook_SessionStart_FirstAnnouncementOfAKnownID_BindsInPlace: a resumed spawn
// (ResumeChat / a switch-back) comes up already inside the conversation it was told to
// resume; announcing it changes nothing.
func TestIngestHook_SessionStart_FirstAnnouncementOfAKnownID_BindsInPlace(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	exchange(t, f, r1, "claude", "content so sA is resumable")
	f.term.exit(t, f.runner(t, r1).TerminalSession) // the CLI exits; chat A goes dormant
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatA)
	require.NoError(t, err)
	f.wait()

	f.announce(t, revived, "sA")

	assert.Equal(t, chatA, f.runner(t, revived).CurrentChatID, "the resumed runner stays on the chat it was spawned into")
	assert.Equal(t, "sA", f.conversation(t, chatA, "claude").SessionID)
	assert.Empty(t, f.markers(t, chatA),
		"the first announcement replaced a session that never turned, and the revive's is the bound one")
	assert.Empty(t, f.term.terminateRequestIDs(), "and nothing is evicted: the runner did not move")
}

func TestIngestHook_SessionStart_SameConversationIsANoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.snapFrames.reset()

	f.announce(t, runnerID, "sid-1")
	f.announce(t, runnerID, "sid-1")

	assert.Empty(t, f.runnerKinds(t), "announcing a conversation issues no runner command at all")
	assert.Empty(t, f.markers(t, chatID), "re-announcing the same conversation changes nothing")
	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
}

func TestIngestHook_SessionStart_EmptySessionID_IsIgnored(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.snapFrames.reset()

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
		mustJSON(t, map[string]any{"last_assistant_message": "done thing"})))

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
		mustJSON(t, map[string]any{})))

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
	assert.Empty(t, f.runnerKinds(t), "a hook must never resurrect a runner we do not know")
}

// TestIngestHook_ExitedRunner_IsIgnored: once the PTY is gone the runner is gone, and a
// late hook from it changes nothing. (Liveness has exactly one source: the process.)
func TestIngestHook_ExitedRunner_IsIgnored(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	f.bc.reset()
	f.snapFrames.reset()

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

	assert.False(t, f.isLive(runnerID), "no runner may outlive its PTY")
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "the chat is dormant")

	// ...but its conversation survives the process, so it stays resumable.
	assert.Equal(t, "s1", f.conversation(t, chatID, "claude").SessionID)
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
// order, and a session change produces neither — it is the chat ledger's alone.
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
	f.announce(t, runnerID, "s2") // /clear: the chat moves onto a new session
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Equal(t, []string{
		"created",              // SpawnChat
		"permission_level_set", // SpawnChat: seeded from the global default
		"title_set",            // user_prompt: derived title
		"turn_started",         // user_prompt: StartTurn
		"turn_stopped",         // turn_stop: StopTurn
		"deleted",              // PurgeChat (asynx Forget's OnForget)
	}, f.bcKinds(t))

	assert.Equal(t, []string{
		"started", // SpawnChat
		"exited",  // PurgeChat
	}, f.runnerKinds(t))
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestSpawnChat_CreateChatFailure_TearsDownCLIAndWraps(t *testing.T) {
	f, cs := newFaultFixture(t)
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
	f := newFixture(t)
	f.refuseRunnerStarted(t)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.ErrorContains(t, err, "record runner")

	require.Equal(t, 1, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-1",
		"a CLI nothing points at must never be left running")
}

// A spawn refused after the chat was written (a CLI exiting at startup, a runner
// row that will not commit) forgets the chat. The frames prove it was created
// first, so the test cannot pass by failing earlier.
func TestRegression_SpawnRefusedAfterTheChatWasWritten_ForgetsTheChat(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(t *testing.T, f testFixture)
	}{
		{
			// The production case: `true` as a vendor CLI — a process that swears it is
			// interactive and is already gone by the time its row could be written.
			name: "the CLI exits during startup",
			arm: func(t *testing.T, f testFixture) {
				t.Helper()
				f.term.duringForkCall = func(call commandCall) { call.onExit(engineterminal.Exit{}) }
			},
		},
		{
			name: "the runner row will not commit",
			arm: func(t *testing.T, f testFixture) {
				t.Helper()
				f.refuseRunnerStarted(t)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.arm(t, f)

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
	f, cs := newFaultFixture(t)

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

	chatID, runnerID := f.spawn(t, "codex")
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
	assert.Equal(t, "codex", convs[0].ProviderID)

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

// The answer is in the ledger before StopTurn publishes Working=false: the chat
// view reads the ledger once on that edge and then stops, so a later append
// would strand the reply.
func TestRegression_TurnStop_AssistantMessageIsDurableBeforeTurnStateIsPublished(t *testing.T) {
	f, cs := newFaultFixture(t)

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
		mustJSON(t, map[string]any{})))
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

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID, "", nil)
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": message, "transcript_path": "/rollouts/x"})))
	f.wait()
	spawnsBefore := f.term.callCount()

	second, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID, "", nil)

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
	kind        string
}

func (r *deltaCallbackRecorder) record(chatID, workspaceID, messageID, text, kind string, _ time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, deltaCall{chatID, workspaceID, messageID, text, kind})
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
		"turn_id":    "turn-1",
		"message_id": messageID,
		"index":      index,
		"final":      final,
		"delta":      text,
	})
}

// Reasoning rides the SAME live channel as the answer, tagged with a kind, and is
// never written to the ledger. codex spends most of a hard turn emitting nothing
// but this, so before it was mapped the chat sat on a spinner and a rotating
// flavour verb for the whole of it.
//
// Payload shape is a LIVE capture against codex-cli 0.149.1 — see
// engine/agents/.../testdata/fixtures/codex/item_reasoning_summaryTextDelta.json.
func TestRegression_ReasoningStreamsLiveAndIsNeverRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	deltas := &deltaCallbackRecorder{}
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{MessageDelta: deltas.record})

	think := func(index int, text string) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, uuid.NewString(), runnerID, "codex", "reasoning_delta",
			mustJSON(t, map[string]any{
				"threadId": "sess-1", "turnId": "turn-1", "itemId": "rs_1",
				"summaryIndex": index, "delta": text,
			})))
	}
	think(0, "**Clarifying** ")
	think(0, "the wording")
	f.wait()

	calls := deltas.snapshot()
	require.Len(t, calls, 2, "every reasoning delta must reach the live channel")
	for _, c := range calls {
		assert.Equal(t, "reasoning", c.kind, "a thought must not arrive tagged as the answer")
		assert.Equal(t, chatID, c.chatID)
		assert.Equal(t, "rs_1", c.messageID)
	}
	assert.Equal(t, "**Clarifying** the wording", calls[1].text,
		"the channel carries the block SO FAR, exactly as the answer channel does")

	// The ledger must be untouched: a thought is a view of a turn in progress,
	// never a record of it. Recording it would put the model's thinking into the
	// transcript as though it had said it out loud.
	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	for _, tn := range turns {
		assert.NotContains(t, tn.Text, "Clarifying",
			"reasoning must never be written to the ledger")
	}
}

// Every streamed delta pushes the cumulative message, so a client that missed a
// frame is right again on the next. fakeCommander has no Screen, so the sweep
// never starts and every call lands on this goroutine.
func TestStartTerminalWaitSweep_PushesEveryDeltaAsTheMessageSoFar(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	deltas := &deltaCallbackRecorder{}
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{MessageDelta: deltas.record})

	post := func(index int, final bool, text string) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, uuid.NewString(), runnerID, "claude", "message_delta",
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

// Two concurrent sends, each staging its own provider on one chat, never
// cross-deliver: each message lands on the provider its own call staged
// (SubmitPromptWithSwitch switches and delivers under one gate).
func TestRegression_SubmitPromptWithStagedProvider_ConcurrentSendsNeverCrossDeliver(t *testing.T) {
	f := newFixture(t)
	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude")

	type outcome struct {
		provider string
		message  string
		result   domain.AgentPromptSubmission
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	launch := func(provider, message string) {
		go func() {
			<-start
			d, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), provider, nil)
			results <- outcome{provider: provider, message: message, result: d, err: err}
		}()
	}
	launch("codex", "message meant for codex")
	launch("claude", "message meant for claude")
	close(start)
	first := <-results
	if first.err == nil {
		// The later send's switch waits for this prompt: the provider takes it and answers.
		prompt(t, f, first.result.RunnerID, first.provider, first.message)
		turn(t, f, first.result.RunnerID, first.provider, "done")
	}
	deadline, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	var second outcome
	select {
	case second = <-results:
	case <-deadline.Done():
		t.Fatal("the later send never finished")
	}
	f.wait()

	startedAs := map[string]string{}
	for _, row := range ledgerRows(t, f, chatID) {
		if row.Kind == ledger.KindRunnerStarted {
			started, err := ledger.DecodeBody[ledger.RunnerStarted](row)
			require.NoError(t, err)
			startedAs[started.RunnerID] = started.Provider
		}
	}
	for _, o := range []outcome{first, second} {
		if o.err != nil {
			// A refusal is never a misdelivery — only a wrong destination is
			// the bug this test exists to catch.
			continue
		}
		assert.Equal(t, o.provider, startedAs[o.result.RunnerID],
			"a staged-provider send must land on the provider IT staged, never a "+
				"concurrent caller's: message=%q landed on provider=%s", o.message, startedAs[o.result.RunnerID])
	}
}

// Targets claude, not codex, for the invalid model: codex's own catalogue is
// now DISCOVERED (model.discover:), so with no live codex CLI in this test
// environment nothing has resolved and Allowed() cannot call any model
// invalid for it yet (see engineagents.Allowed) — claude's is still a
// static, always-resolved list, so it stays the reliable vehicle for "the
// selection step itself fails" this regression is actually about.
func TestRegression_SubmitPromptWithStagedProviderAndInvalidModel_SwitchStaysCommitted(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "claude", &domain.ChatSelection{Model: "not-a-real-model", Effort: ""})

	require.Error(t, err)
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	t.Logf("after invalid-model SubmitPrompt with a staged switch: err=%v liveProvider=%s working=%v",
		err, live.ProviderID, f.chat(t, chatID).Working)
	assert.Equal(t, "codex", live.ProviderID,
		"a switch committed by a call whose OWN selection step then fails should not silently "+
			"strand the chat on the new provider with no prompt delivered and no way back but another switch")
}

// A send the gate turns away must not leave its staged model/effort behind:
// the NEXT send would otherwise restart the provider for a pick that was
// never delivered.
func TestRegression_SubmitPromptWithStagedSelection_BusyRejectionChangesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", uuid.NewString(), "", nil)
	require.NoError(t, err)
	before := f.chat(t, chatID)

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "two", uuid.NewString(), "",
		&domain.ChatSelection{Model: "staged-model", Effort: "high"})

	require.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	after := f.chat(t, chatID)
	assert.Equal(t, before.Model, after.Model)
	assert.Equal(t, before.Effort, after.Effort)
}

func TestRegression_SubmitPromptWithStagedSelection_ReplayedRequestChangesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", requestID, "", nil)
	require.NoError(t, err)
	before := f.chat(t, chatID)

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "one", requestID, "",
		&domain.ChatSelection{Model: "staged-model", Effort: "high"})

	require.NoError(t, err)
	after := f.chat(t, chatID)
	assert.Equal(t, before.Model, after.Model)
	assert.Equal(t, before.Effort, after.Effort)
}

// ─── the prompt a departing runner leaves behind ──────────────

// TestRegression_AChatWhoseCLIClearedIsNotLeftOwingAPrompt: a CLI that moves the
// chat onto a new session can no longer acknowledge the prompt it carried in the
// old one, so the change must settle it — else the chat answers 409 to every
// later prompt.
func TestRegression_AChatWhoseCLIClearedIsNotLeftOwingAPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "conversation-one")
	exchange(t, f, runnerID, "codex", "hello")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "can you call some subagents?", uuid.NewString(), "", nil)
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	f.announce(t, live.ID, "conversation-one") // the delivery resumed it
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the delivery is spawned and nothing has confirmed it")

	// The user typed /clear: the CLI moves this chat onto a session nobody has seen.
	f.announce(t, live.ID, "conversation-three")
	require.Equal(t, chatID, f.runner(t, live.ID).CurrentChatID, "precondition: the runner stayed")

	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the change must settle the delivery the old session took with it")
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "and now?", uuid.NewString(), "", nil)
	require.NoError(t, err, "the chat must not be left owing an answer nobody can give")
}

// TestRegression_DormantChatWithAPromptFromAVanishedRunnerStillResumes: a
// departure that never settled its prompt leaves one no runner can acknowledge.
// The pending check settles it rather than refuse every resume for good.
func TestRegression_DormantChatWithAPromptFromAVanishedRunnerStillResumes(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "conversation-one")
	turn(t, f, runnerID, "codex", "hello")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	orphan := plantPrompt(t, f, chatID, "a-runner-that-no-longer-exists")

	_, err := f.usecase.ResumeChat(f.ctx, chatID)

	require.NoError(t, err, "a prompt whose carrier is gone must not refuse the resume forever")
	got := receipt(t, f, chatID, orphan)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonRunnerExited, got.Reason)
}

// The same orphan must not hold a live chat busy for its next prompt either.
func TestRegression_APromptFromAVanishedRunnerDoesNotHoldTheNextPromptBusy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	orphan := plantPrompt(t, f, chatID, "a-runner-that-no-longer-exists")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "and now?", uuid.NewString(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, ledger.ReasonRunnerExited, receipt(t, f, chatID, orphan).Reason)
}

// ---------------------------------------------------------------------------
// A chat never changes provider without being asked
// ---------------------------------------------------------------------------

// Re-opening a dormant chat born on codex keeps codex: it wrote no conversation
// row and no switch marker, and resolving that absence to the first enabled
// provider changed the chat on a click.
func TestRegression_ReopeningADormantChatBornOnCodexKeepsCodex(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	// No f.announce: nothing ever binds a conversation row for this chat, and
	// there is no earlier provider's row to inherit either.
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")
	convs, err := f.usecase.ConversationsForChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Empty(t, convs, "precondition: codex bound nothing")
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Empty(t, ints, "precondition: a chat born on codex carries no switch marker")

	// The chat itself is the only thing left that knows.
	assert.Equal(t, "codex", f.chat(t, chatID).ProviderID,
		"a chat must remember the vendor it was born on, with or without a runner on it")

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err,
		"re-opening this chat must revive it, not fail and leave the UI to guess a provider")
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "codex", live.ProviderID,
		"the chat came back as codex — a reopen that converts it to another vendor is the bug")
}

// TestRegression_SwitchingBackToTheChatsOwnProviderKeepsRecordingTheMarker
// closes the OTHER half of "silently": a switch whose previous provider could
// not be resolved recorded no provider_switched marker at all, so a conversion
// left no trace anywhere. The durable choice is what makes the previous
// provider knowable, which is what makes a real conversion legible.
func TestRegression_SwitchingBackToTheChatsOwnProviderKeepsRecordingTheMarker(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1, "a real conversion must leave a breadcrumb even with nothing bound")
	assert.Equal(t, agents.InterruptProviderSwitched, ints[0].Kind)
	assert.Equal(t, "claude", ints[0].Detail)
	assert.Equal(t, "claude", f.chat(t, chatID).ProviderID,
		"and the chat's own durable choice moves with the switch the user asked for")
}

// TestRegression_SwitchProvider_ConvertingAChatWithNoResolvableProviderStillLeavesAMarker
// is the other half of "silently". The switch marker was gated on the PREVIOUS
// provider being resolvable, so a chat neither projection could name recorded
// nothing at all: the conversion happened and left no trace anywhere — no
// divider in the transcript, and no interruption for the next resolver to read.
//
// The chat here is the one shape that can still reach that state: a row minted
// before domain.Chat.ProviderID existed, which carries "" and has nothing bound.
// Switching it is legitimate — hand-picking a vendor is the only way left to
// name one — but it must be recorded.
func TestRegression_SwitchProvider_ConvertingAChatWithNoResolvableProviderStillLeavesAMarker(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
	require.NoError(t, err)
	f.wait()
	require.Empty(t, f.chat(t, chatID).ProviderID,
		"precondition: nothing about this chat names a vendor, exactly like a pre-field row")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1, "a conversion with an unknown starting point is still a conversion")
	assert.Equal(t, agents.InterruptProviderSwitched, ints[0].Kind)
	assert.Equal(t, "claude", ints[0].Detail)
}

// A revive resumes the chat's own session by its id, and the chat says which one.
func TestResumeBySessionID_ARevivedChatResumesItsBoundSession(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-kept")
	exchange(t, f, runnerID, "claude", "kept content")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "sid-kept", argAfter(t, argv, "--resume"))
	snap, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	c, ok := snap.Conversation()
	require.True(t, ok)
	assert.Equal(t, "sid-kept", c.SessionID)
}

// A session the chat never turned on was never written by its CLI: every launch
// until it turns is assigned a new id, never its old one and never resumed.
func TestResumeBySessionID_ASessionThatNeverTurnedIsReplacedNotResumed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	spawned := argAfter(t, f.term.calls[0].argv, "--session-id")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	argv := f.term.calls[1].argv
	revived := argAfter(t, argv, "--session-id")
	assert.NotEqual(t, spawned, revived, "a revive never reuses an id its CLI never wrote")
	assert.NotContains(t, argv, "--resume", "nor resumes it")

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "first words", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[2].argv
	delivering := argAfter(t, argv, "--session-id")
	assert.NotContains(t, []string{spawned, revived}, delivering)
	assert.NotContains(t, argv, "--resume")
	c := f.conversation(t, chatID, "claude")
	assert.Equal(t, ledger.CauseAssigned, c.Cause)

	hook(t, f, first.RunnerID, "claude", "user_prompt", map[string]any{"prompt": "first words", "session_id": delivering})
	turn(t, f, first.RunnerID, "claude", "answered")
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "second words", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, delivering, argAfter(t, argv, "--resume"), "once it turned, it is resumed")
	assert.NotContains(t, argv, "--session-id")
}

// A provider that takes no id at launch starts fresh on a session that never
// turned; its announcement rebinds the chat with no change, so the prompt the
// launch carries stays owed and is delivered.
func TestResumeBySessionID_AnUnturnedSessionOfAProviderThatTakesNoIDLaunchesFresh(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-unturned")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	requestID := uuid.NewString()
	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello codex", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.NotContains(t, argv, "resume")
	assert.NotContains(t, argv, "thread-unturned")

	f.announce(t, result.RunnerID, "thread-fresh")
	assert.Equal(t, "thread-fresh", f.conversation(t, chatID, "codex").SessionID)
	assert.Empty(t, f.markers(t, chatID), "leaving a session that never turned is no change")
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the prompt this launch carries is still owed")

	prompt(t, f, result.RunnerID, "codex", "hello codex")
	assert.Equal(t, ledger.PromptStatusDelivered, receipt(t, f, chatID, requestID).Status)
}

// A resume the CLI itself refuses loses the session: the chat says why its runner
// ended, and the next launch starts a fresh session with no transcript.
func TestResumeBySessionID_ARefusedResumeLosesTheSession(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-corrupt")
	exchange(t, f, runnerID, "claude", "before the corruption")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	require.Equal(t, "sid-corrupt", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"))
	f.term.exitWith(t, f.runner(t, revived).TerminalSession, engineterminal.Exit{
		Code: 1, Screen: "No conversation found with session ID: sid-corrupt",
	})
	f.wait()

	assert.Equal(t, domain.AgentExitResumeFailed, agentusecase.ChatSession(f.usecase.RunnerUsecase, chatID).ExitReason)
	assert.Equal(t, ledger.ConversationLost, f.conversation(t, chatID, "claude").State)

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.NotContains(t, argv, "--resume", "a lost session is never resumed")
	fresh := f.conversation(t, chatID, "claude")
	assert.Equal(t, ledger.ConversationBound, fresh.State)
	assert.Equal(t, fresh.SessionID, argAfter(t, argv, "--session-id"))
	assert.NotContains(t, strings.Join(argv, "\x00"), "before the corruption", "no transcript is handed over")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "after the loss", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[f.term.callCount()-1].argv
	assert.NotEqual(t, fresh.SessionID, argAfter(t, argv, "--session-id"), "the fresh session never turned")
	assert.Equal(t, ledger.CauseFresh, f.conversation(t, chatID, "claude").Cause, "and its replacement is fresh too")
	assert.Contains(t, strings.Join(argv, "\x00"), "after the loss")
	assert.NotContains(t, strings.Join(argv, "\x00"), "before the corruption", "nor to the launch carrying the prompt")
}

// The prompt that launched a resume the CLI refuses is lost with the session,
// and nothing launches again to redeliver it.
func TestResumeBySessionID_APromptCarriedIntoARefusedResumeIsLostWithIt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-corrupt")
	exchange(t, f, runnerID, "claude", "an answer")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	requestID := uuid.NewString()
	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "lost with it", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	require.Equal(t, "sid-corrupt", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"))
	calls := f.term.callCount()
	f.term.exitWith(t, result.TerminalSessionID, engineterminal.Exit{
		Code: 1, Screen: "No conversation found with session ID: sid-corrupt",
	})
	f.wait()

	got := receipt(t, f, chatID, requestID)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonSessionLost, got.Reason)
	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID))
	assert.Equal(t, calls, f.term.callCount(), "the lost prompt is not redelivered")
}

// A resumed CLI Crowbar stops did not refuse its session: the chat says it was
// stopped, and the next revive resumes it.
func TestResumeBySessionID_AResumeStoppedBeforeItSpeaksIsNotARefusal(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-kept")
	exchange(t, f, runnerID, "claude", "an answer")
	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	require.Equal(t, "sid-kept", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"))

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, result.TerminalSessionID) // the stopped CLI goes
	f.wait()
	assert.Equal(t, domain.AgentExitStopped, agentusecase.ChatSession(f.usecase.RunnerUsecase, chatID).ExitReason)

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	assert.Equal(t, "sid-kept", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"),
		"a session nobody refused is never lost")
}

// A replacement that cannot start leaves the chat dormant saying so — never a
// silent dormancy, and never the "displaced" of the runner it replaced.
func TestSessionExit_AReplacementThatCannotStartRecordsSpawnFailed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-before")
	turn(t, f, runnerID, "claude", "an answer")
	f.term.err = errors.New("pty: out of descriptors")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	require.Error(t, err)

	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
	assert.Equal(t, domain.AgentExitSpawnFailed, agentusecase.ChatSession(f.usecase.RunnerUsecase, chatID).ExitReason)
}

// sessionOp is one step of the invariant walk: anything a user, a CLI or the
// vendor's own store can do to a chat.
type sessionOp struct {
	name string
	do   func(t *testing.T, f testFixture, w *sessionWalk)
}

// sessionWalk is the chat the walk is on and what it has seen so far.
type sessionWalk struct {
	chatID   string
	provider string
	sessions int
	// rival is the intent racing this one, when two run at once.
	rival string
}

func (w *sessionWalk) live(t *testing.T, f testFixture) (agents.Runner, bool) {
	t.Helper()
	r, err := f.liveRunnerFor(t, w.chatID)
	return r, err == nil
}

var sessionOps = []sessionOp{
	{"send", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		sub, err := f.usecase.SubmitPrompt(f.ctx, w.chatID, "step", uuid.NewString(), "", nil)
		if err != nil {
			return
		}
		// S5: a delivered send has a runner to continue the conversation on —
		// unless a rival stop or crash took the chat once the send released it.
		assert.NotEmpty(t, sub.RunnerID, "S5: a delivered send names no runner")
		live, ok := w.live(t, f)
		assert.True(t, ok || endsRunner(w.rival), "S5: a delivered send left the chat with no runner")
		// The CLI takes what it is sent; a switch parked on the prompt waits for that.
		if ok && live.ID == sub.RunnerID {
			prompt(t, f, live.ID, live.ProviderID, "step")
		}
	}},
	{"stop", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		// S4: Stop is bounded and leaves no turn open.
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			_ = f.usecase.StopChat(f.ctx, w.chatID)
		}()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Fatal("S4: Stop did not return")
		}
		f.wait()
		assert.False(t, f.chat(t, w.chatID).Working, "S4: a stopped chat is left working")
	}},
	{"switch", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		w.provider = map[string]string{"claude": "codex", "codex": "claude"}[w.provider]
		_, _ = f.usecase.SwitchProvider(f.ctx, w.chatID, w.provider)
	}},
	{"resume", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		_, _ = f.usecase.ResumeChat(f.ctx, w.chatID)
	}},
	{"crash", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if r, ok := w.live(t, f); ok && r.TerminalSession != "" && f.term.SessionLive(f.ctx, r.TerminalSession) {
			f.term.exit(t, r.TerminalSession)
		}
	}},
	{"restart", func(t *testing.T, f testFixture, _ *sessionWalk) {
		t.Helper()
		f.restart(t)
		require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	}},
	{"announce", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if r, ok := w.live(t, f); ok {
			w.sessions++
			f.announce(t, r.ID, fmt.Sprintf("walk-%s-%d", w.chatID[:8], w.sessions))
		}
	}},
	{"prompt", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		// A turn the CLI opened and has not finished: whatever comes next must close it.
		if r, ok := w.live(t, f); ok && f.sessions[r.ID] != "" {
			payload := map[string]any{"prompt": "typed in the terminal"}
			require.NoError(t, f.usecase.IngestHook(f.ctx, r.ID, r.ProviderID, "user_prompt", mustJSON(t, payload)))
		}
	}},
	{"turn", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if r, ok := w.live(t, f); ok && f.sessions[r.ID] != "" {
			turn(t, f, r.ID, r.ProviderID, "an answer")
		}
	}},
	{"refuse", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		// The CLI dies refusing its session; it loses it only if it was resuming one.
		if r, ok := w.live(t, f); ok && r.TerminalSession != "" && f.term.SessionLive(f.ctx, r.TerminalSession) {
			f.term.exitWith(t, r.TerminalSession, engineterminal.Exit{
				Code: 1, Screen: "No conversation found with session ID\nError: no rollout found",
			})
		}
	}},
	{"delete", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if err := f.usecase.PurgeChat(f.ctx, w.chatID); err != nil {
			return
		}
		f.wait()
		// S8: nothing kept for the chat outlives it.
		rows, err := f.ledger.Read(f.ctx, w.chatID, 0, 0)
		require.NoError(t, err)
		assert.Empty(t, rows)
		w.chatID, _ = f.spawn(t, w.provider)
	}},
}

// checkSessionInvariants asserts S1 and S2 after every step of the walk.
func checkSessionInvariants(t *testing.T, f testFixture, w *sessionWalk, trail []string) {
	t.Helper()
	// S1: one runner per vendor session (one per chat is the live table's key).
	all := agentusecase.AllLiveRunners(f.usecase.own)
	holders := map[string]int{}
	for _, r := range all {
		if c := f.conversation(t, r.CurrentChatID, r.ProviderID); r.CurrentChatID != "" && c.SessionID != "" {
			holders[r.ProviderID+"/"+c.SessionID]++
		}
	}
	for session, n := range holders {
		assert.Equal(t, 1, n, "S1 session %s, after %v", session, trail)
	}
	// S2: a chat with no runner says why its last one ended.
	// S3: and has no turn left open — nothing is left that could close it.
	if _, ok := w.live(t, f); !ok {
		assert.NotEmpty(t, agentusecase.ChatSession(f.usecase.RunnerUsecase, w.chatID).ExitReason,
			"S2, after %v", trail)
		assert.False(t, f.chat(t, w.chatID).Working, "S3, after %v", trail)
	}
}

// concurrentOps are the user's own intents, which really do race each other
// from different tabs and clients.
var concurrentOps = []string{"send", "stop", "switch", "resume", "crash"}

// endsRunner reports whether an intent may leave the chat with no runner.
func endsRunner(op string) bool { return op == "stop" || op == "crash" }

func sessionOpNamed(name string) sessionOp {
	for _, op := range sessionOps {
		if op.name == name {
			return op
		}
	}
	panic("no session op " + name)
}

// Two intents at once — a send racing a stop, a switch racing a resume — leave
// the same invariants standing. Run under -race.
func TestSessionInvariants_HoldWhenIntentsRace(t *testing.T) {
	for seed := uint64(1); seed <= 10; seed++ {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := newFixture(t).withSwitchGrace(t, 10*time.Millisecond)
			rng := rand.New(rand.NewPCG(seed, seed))
			w := &sessionWalk{provider: "claude"}
			var runnerID string
			w.chatID, runnerID = f.spawn(t, w.provider)
			f.announce(t, runnerID, "race-"+w.chatID[:8])
			var trail []string
			for range 20 {
				a := sessionOpNamed(concurrentOps[rng.IntN(len(concurrentOps))])
				b := sessionOpNamed(concurrentOps[rng.IntN(len(concurrentOps))])
				trail = append(trail, a.name+"|"+b.name)
				// Each goroutine reads its own copy: a switch flips the walk's provider.
				wa, wb := *w, *w
				wa.rival, wb.rival = b.name, a.name
				var both sync.WaitGroup
				both.Add(2)
				go func() { defer both.Done(); a.do(t, f, &wa) }()
				go func() { defer both.Done(); b.do(t, f, &wb) }()
				both.Wait()
				f.wait()
				if live, ok := w.live(t, f); ok {
					w.provider = live.ProviderID
				}
				checkSessionInvariants(t, f, w, trail)
				if t.Failed() {
					return
				}
			}
		})
	}
}

// The send|stop interleaving the race walk hits: the send holds the chat's
// spawn gate until it has delivered, so a racing Stop takes the chat only after
// it — and ends the runner the send was delivered to, saying why.
func TestSessionInvariants_AStopAfterADeliveredSendEndsItsRunnerAndSaysWhy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-send-stop")

	sub, err := f.usecase.SubmitPrompt(f.ctx, chatID, "step", uuid.NewString(), "", nil)
	require.NoError(t, err)
	delivered, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, sub.RunnerID, delivered.ID, "the send names the runner it was delivered to")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
	assert.Equal(t, domain.AgentExitStopped, agentusecase.ChatSession(f.usecase.RunnerUsecase, chatID).ExitReason)
	assert.False(t, f.chat(t, chatID).Working)
}

// The session invariants hold under any interleaving of what users, CLIs and
// vendor stores do (sessions plan §3), walked from fixed seeds so a failure
// replays exactly.
func TestSessionInvariants_HoldUnderRandomInterleavings(t *testing.T) {
	for seed := uint64(1); seed <= 10; seed++ {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := newFixture(t).withSwitchGrace(t, 10*time.Millisecond)
			rng := rand.New(rand.NewPCG(seed, seed))
			w := &sessionWalk{provider: "claude"}
			w.chatID, _ = f.spawn(t, w.provider)
			var trail []string
			for range 40 {
				op := sessionOps[rng.IntN(len(sessionOps))]
				trail = append(trail, op.name)
				op.do(t, f, w)
				f.wait()
				checkSessionInvariants(t, f, w, trail)
				if t.Failed() {
					return
				}
			}
		})
	}
}
