package agent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func TestSwitchProvider_TerminatesOutgoingTerminal_AndEndsOldSegment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	oldSeg := activeSegOf(t, f.chat(t, chatID), segID)
	require.NotEmpty(t, oldSeg.TerminalSessionID)

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	assert.Contains(t, f.term.terminatedIDs(), oldSeg.TerminalSessionID)

	ended := segByID(t, f.chat(t, chatID), segID)
	assert.Equal(t, "ended", ended.Status)
	require.NotNil(t, ended.EndedAt)
}

// TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch: when
// TerminateGraceful fails because the terminal session is already gone (the one
// error the real engine returns today), the switch must still proceed.
func TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newSegID)

	chat := f.chat(t, chatID)
	assert.Equal(t, "ended", segByID(t, chat, segID).Status)
	assert.Equal(t, newSegID, chat.ActiveSegmentID)
}

// TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch: a
// TerminateGraceful failure that is NOT "session already gone" must abort the
// switch entirely rather than spawn a second live CLI into the same worktree.
func TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminate outgoing terminal")

	chat := f.chat(t, chatID)
	assert.Equal(t, "active", segByID(t, chat, segID).Status, "old segment must NOT be ended when terminate genuinely failed")
	require.Equal(t, 1, f.term.callCount(), "no new segment/terminal should have been spawned after a real terminate failure")
	assert.Equal(t, segID, chat.ActiveSegmentID, "active segment must be unchanged")
}

// TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate: AssembleHandoff
// runs BEFORE terminate, so a failure there (here a worktree-dir lookup failure
// inside AssembleHandoff) must leave the chat completely untouched.
func TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.ws.err = errors.New("boom: worktree lookup")
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assemble handoff")

	f.ws.err = nil // let the assertion reads resolve the worktree again
	assert.Empty(t, f.term.terminatedIDs(), "the outgoing terminal must never be terminated when handoff assembly fails first")
	assert.Equal(t, "active", segByID(t, f.chat(t, chatID), segID).Status, "old segment must be untouched")
	require.Equal(t, 1, f.term.callCount(), "no new segment/terminal should have been spawned")
}

func TestSwitchProvider_Forward_SpawnsTargetProviderWithHandoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	appendAssistantTurn(t, f, segID, "claude", "sid-1", "prior turn content for handoff")

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newSegID)

	require.Equal(t, 2, f.term.callCount())
	newCall := f.term.calls[1]
	assert.Equal(t, "codex", newCall.argv[0])
	assert.Contains(t, strings.Join(newCall.argv, "\x00"), "prior turn content for handoff")
}

func TestSwitchProvider_PersistsNewActiveSegmentForTargetProvider(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	chat := f.chat(t, chatID)
	newSeg := segByID(t, chat, newSegID)
	assert.Equal(t, "codex", newSeg.ProviderID)
	assert.Equal(t, "active", newSeg.Status)
	assert.NotEmpty(t, newSeg.TerminalSessionID)
	assert.Equal(t, newSegID, chat.ActiveSegmentID)
}

// TestSwitchProvider_Broadcasts_SegmentEndedThenOpened: a provider switch ends
// the outgoing segment and opens the incoming one, each a distinct aggregate
// event, so the hub fans out exactly two frames in that order (segment_ended
// then segment_opened) — no bespoke "switched" kind, no double-broadcast.
func TestSwitchProvider_Broadcasts_SegmentEndedThenOpened(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset()

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	assert.Equal(t, []string{"segment_ended", "segment_opened"}, f.bcKinds(t))
}

// TestSwitchProvider_SwitchBack_ResumesNativeSessionWithSeparateArgvTokens
// drives forward+back: spawn claude, bind its native session, switch to codex,
// switch back to claude. The switch-back resumes the prior claude session by
// expanding+tokenizing descriptor.Session.Resume.Arg ("--resume {id}") into two
// SEPARATE argv tokens.
func TestSwitchProvider_SwitchBack_ResumesNativeSessionWithSeparateArgvTokens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-claude-native",
	})))

	boundSeg := activeSegOf(t, f.chat(t, chatID), segID)
	require.Equal(t, "sid-claude-native", boundSeg.ProviderSessionID)

	// A session id is not a conversation: the CLI only WRITES one once it has said
	// something, so a session is only resumable after a real turn.
	appendAssistantTurn(t, f, segID, "claude", "sid-claude-native", "claude said something")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	require.Equal(t, 2, f.term.callCount())

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newSeg := segByID(t, f.chat(t, chatID), newSegID)
	assert.Equal(t, "claude", newSeg.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	resumeIdx := indexOf(argv, "--resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain --resume as its own token", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-claude-native", argv[resumeIdx+1])

	assert.NotContains(t, argv, "--resume sid-claude-native")
}

// TestSwitchProvider_SwitchBack_ResumesWithGapOnly exercises the codex-target
// switch-back path end to end. Two things are load-bearing:
//
//   - the resume arg ("resume {id}", no leading dash) MUST precede the positional
//     context, or codex parses the context as its subcommand;
//   - a provider resumed into its OWN session gets the GAP, not the whole ledger.
//     Its native session already replays everything it said before it was switched
//     out, so re-handing it its own turns is pure noise — what it does NOT have is
//     what happened under the other provider while it was away.
func TestSwitchProvider_SwitchBack_ResumesWithGapOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "codex")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "codex", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-codex-native",
	})))
	appendAssistantTurn(t, f, segID, "codex", "sid-codex-native", "codex ledger content")

	claudeSegID, err := f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	// What codex misses while it is away: claude's turn, recorded after codex's
	// segment ended.
	appendAssistantTurn(t, f, claudeSegID, "claude", "sid-claude-native", "claude spoke while codex was away")

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	newSeg := segByID(t, f.chat(t, chatID), newSegID)
	assert.Equal(t, "codex", newSeg.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain resume as its own token", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-codex-native", argv[resumeIdx+1])

	gapIdx := -1
	for i, a := range argv {
		if i > resumeIdx+1 && strings.Contains(a, "claude spoke while codex was away") {
			gapIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, gapIdx, 0, "argv %v must carry the gap after the resume arg", argv)

	// Codex's OWN prior turn is already in the session it resumes — handing it
	// back would duplicate its history and bury the one thing that IS new.
	assert.NotContains(t, argv[gapIdx], "codex ledger content",
		"a resumed provider must not be re-fed its own turns")
}

// TestSwitchProvider_ForwardSwitch_CarriesWholeConversation is the other half of
// the gap rule: a provider that has never run in this chat has no session to
// resume and therefore no history at all, so it gets the ENTIRE ledger.
func TestSwitchProvider_ForwardSwitch_CarriesWholeConversation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-claude-native",
	})))
	appendAssistantTurn(t, f, segID, "claude", "sid-claude-native", "claude said this first")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv

	// codex is new to this chat: no resume, and the context rides the silent
	// developer_instructions channel — never the positional user prompt.
	assert.Equal(t, -1, indexOf(argv, "resume"), "a provider new to the chat has no session to resume")

	doc := argAfter(t, argv, "-c")
	require.True(t, strings.HasPrefix(doc, "developer_instructions="))
	assert.Contains(t, doc, "claude said this first")
}

func TestSwitchProvider_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.usecase.SwitchProvider(ctx, "does-not-exist", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: chat")
}

// TestSwitchProvider_DeadChat_SwitchesAnyway: a chat whose CLI is gone (it
// exited, or it died with the daemon) has no active segment — and that used to
// be a hard dead end. The pane told the user to "switch provider below to start
// a new one" while this call returned ErrNotFound, so the chat could never be
// re-entered by ANY route. An absent active segment now just means there is no
// outgoing CLI to terminate.
func TestSwitchProvider_DeadChat_SwitchesAnyway(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.repo.Create(ctx, agentchat.CreateInput{
		ID: "c1", WorkspaceID: "ws1", SegmentID: "s1", CrowbarSegmentID: "cs1", ProviderID: "claude", TerminalSession: "term-x",
	})
	require.NoError(t, err)
	_, err = f.repo.EndSegment(ctx, "c1", "s1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	f.wait()

	newSegID, err := f.usecase.SwitchProvider(ctx, "c1", "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newSegID)

	newSeg := segByID(t, f.chat(t, "c1"), newSegID)
	assert.Equal(t, "codex", newSeg.ProviderID)
	assert.Equal(t, "active", newSeg.Status)

	// Nothing was alive, so nothing was terminated.
	assert.Empty(t, f.term.terminateRequestIDs(), "a dead chat has no CLI to terminate")
}

// TestResumeChat_RevivesLastProviderIntoItsOwnSession is bug #1: the CLI died
// (here: the daemon restarted and ReconcileOnBoot ended the segment), and the
// chat must come back exactly where the user left it. Everything needed is
// already on the ended segment — its provider, and the native session id the CLI
// bound — so reviving is nothing more than "switch to the provider that was last
// here", which finds that session id and resumes into it.
func TestResumeChat_RevivesLastProviderIntoItsOwnSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-claude-native",
	})))
	appendAssistantTurn(t, f, segID, "claude", "sid-claude-native", "claude said something")
	f.wait()

	// The CLI dies (daemon restart / process exit): the segment ends.
	chat := f.chat(t, chatID)
	_, err = f.repo.EndSegment(ctx, chatID, chat.ActiveSegmentID, time.Now())
	require.NoError(t, err)
	f.wait()
	require.Empty(t, f.chat(t, chatID).ActiveSegmentID)

	newSegID, err := f.usecase.ResumeChat(ctx, chatID)
	require.NoError(t, err)

	newSeg := segByID(t, f.chat(t, chatID), newSegID)
	assert.Equal(t, "claude", newSeg.ProviderID, "revive must bring back the provider that was last here")
	assert.Equal(t, "active", newSeg.Status)

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv
	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"revive must resume the CLI's own native session, not start a blank one")

	// Nothing happened while it was gone, so it is handed NO conversation at all —
	// its own session already holds every turn. (The chat is still untitled, so the
	// title instruction rides along; that is the only thing in the document.)
	doc := argAfter(t, argv, "--append-system-prompt")
	assert.NotContains(t, doc, "WHILE YOU WERE AWAY",
		"a revive with an empty gap must hand over no conversation")
	assert.NotContains(t, doc, "HANDED-OFF CONTEXT",
		"a revived provider must never be re-fed the conversation it already has")
}

// TestResumeChat_LiveChat_IsNoop: reviving a chat whose CLI is alive must never
// tear that CLI down — it just hands back the segment already running.
func TestResumeChat_LiveChat_IsNoop(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	got, err := f.usecase.ResumeChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, segID, got)
	assert.Equal(t, 1, f.term.callCount(), "a live chat must not respawn its CLI")
	assert.Empty(t, f.term.terminateRequestIDs())
}

func TestSwitchProvider_WorkspaceReaderFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	// A workspace-reader failure surfaces wrapped, not swallowed. SwitchProvider's
	// first ws-reader call is AssembleHandoff resolving the chats dir (the ledger
	// now lives under the workspace's chats dir, rerooted under home for a home-kind
	// workspace), so the surfaced wrap is "chats dir", not "worktree dir".
	f.ws.err = errors.New("boom: workspace lookup")
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chats dir")
}

func TestSwitchProvider_UnknownTargetProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.SwitchProvider(ctx, chatID, "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
}

func TestSwitchProvider_EndOldSegmentFailure_ReturnsWrappedError(t *testing.T) {
	f, fs := newFaultFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	fs.failEndSeg = errors.New("boom: end segment")
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: end old segment")
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

// TestResumeChat_SessionWithNoTurns_SpawnsFreshInsteadOfResumingAPhantom is the
// regression for a bug that reached the running app: opening a chat the user had
// never sent a message in killed it outright, with claude printing
//
//	No conversation found with session ID: dc4b2ff8-…
//
// A session id is NOT a conversation. Every CLI reports its id the instant it
// starts (that is when our SessionStart hook records it), but only WRITES the
// conversation once there is at least one message — so a chat that was opened and
// never used has an id pointing at nothing, and resuming it fails on startup.
// Crowbar records a turn from the very same hooks, so an empty ledger for that
// session is the proof that there is nothing to resume: spawn fresh instead.
func TestResumeChat_SessionWithNoTurns_SpawnsFreshInsteadOfResumingAPhantom(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	// The CLI came up and reported its session id — but the user never typed, so no
	// turn was ever recorded and claude never wrote this conversation to disk.
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-never-persisted",
	})))
	f.wait()

	chat := f.chat(t, chatID)
	require.Equal(t, "sid-never-persisted", segByID(t, chat, chat.ActiveSegmentID).ProviderSessionID)

	_, err = f.repo.EndSegment(ctx, chatID, chat.ActiveSegmentID, time.Now())
	require.NoError(t, err)
	f.wait()

	newSegID, err := f.usecase.ResumeChat(ctx, chatID)
	require.NoError(t, err)

	assert.Equal(t, "claude", segByID(t, f.chat(t, chatID), newSegID).ProviderID)
	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv

	assert.Equal(t, -1, indexOf(argv, "--resume"),
		"a session the CLI never wrote must NOT be resumed — claude dies with "+
			"\"No conversation found with session ID\"; argv was %v", argv)
	for _, a := range argv {
		assert.NotContains(t, a, "sid-never-persisted")
	}
}

// TestSwitchProvider_SwitchBackToProviderWithNoTurns_DoesNotResume: same rule on
// the switch-back path. A provider that ran in this chat but never said anything
// has no session to return to, so it is spawned fresh — and, having no history of
// its own, it gets the WHOLE conversation rather than a gap.
func TestSwitchProvider_SwitchBackToProviderWithNoTurns_DoesNotResume(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, claudeSeg, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	// claude binds a session but never takes a turn.
	require.NoError(t, f.usecase.IngestHook(ctx, claudeSeg, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-claude-empty",
	})))
	f.wait()

	codexSeg, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	appendAssistantTurn(t, f, codexSeg, "codex", "sid-codex", "codex actually said something")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	assert.Equal(t, -1, indexOf(argv, "--resume"), "argv %v must not resume a session with no conversation", argv)
	// No session of its own → it is new to the conversation → it gets all of it.
	doc := argAfter(t, argv, "--append-system-prompt")
	assert.Contains(t, doc, "codex actually said something")
	assert.Contains(t, doc, "HANDED-OFF CONTEXT")
}
