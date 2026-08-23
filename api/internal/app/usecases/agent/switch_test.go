package agent_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
)

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

// TestSwitchProvider_SwitchBack_ResumesAndPointsAtTheGap exercises the codex-target
// switch-back path. Two things are load-bearing:
//
//   - the resume arg ("resume {id}", no leading dash) MUST precede the positional, or
//     codex parses the message as its subcommand;
//   - a resumed codex can only be reached through a USER MESSAGE, so it is handed a
//     POINTER — the ledger directory plus the last turn it already saw — and NOT the
//     transcript. Pasting the handed-off exchange into the chat is a wall of text the
//     user has to scroll past on every switch, and the agent can just read the file.
func TestSwitchProvider_SwitchBack_ResumesAndPointsAtTheGap(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner := f.spawn(t, "codex")
	f.announce(t, codexRunner, "sid-codex-native")
	turn(t, f, codexRunner, "codex", "codex ledger content")

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
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain resume as its own token", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-codex-native", argv[resumeIdx+1])

	msg := argv[len(argv)-1]
	assert.Contains(t, msg, "[Crowbar]")
	assert.Contains(t, msg, "get_chat_log", "the message must name the tool that reads the record: %q", msg)
	assert.Contains(t, msg, "limit 1", "the message must name HOW MUCH is new, or the CLI re-reads what it was already handed: %q", msg)

	// The transcript itself must NOT be in the message — neither the gap nor its own
	// earlier turns. That is the whole point: point at the record, do not paste it.
	assert.NotContains(t, msg, "claude spoke while codex was away")
	assert.NotContains(t, msg, "codex ledger content")
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

	// A workspace-reader failure surfaces wrapped, not swallowed.
	f.ws.err = errors.New("boom: workspace lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chats dir")
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
	// session store to lose in between.
	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must resume codex's own conversation", argv)
	assert.Equal(t, "sid-codex", argv[resumeIdx+1])
}
