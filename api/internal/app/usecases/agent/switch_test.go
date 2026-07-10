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

// TestSwitchProvider_SwitchBack_ResumeStepsPrecedeHandoff exercises the
// codex-target switch-back path, where the resume arg is "resume {id}" (no
// leading dash) and MUST precede the positional handoff arg.
func TestSwitchProvider_SwitchBack_ResumeStepsPrecedeHandoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "codex")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "codex", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-codex-native",
	})))

	appendAssistantTurn(t, f, segID, "codex", "sid-codex-native", "codex ledger content")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

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

	handoffIdx := -1
	for i, a := range argv {
		if i > resumeIdx+1 && strings.Contains(a, "codex ledger content") {
			handoffIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, handoffIdx, 0, "argv %v must contain the handoff content after resume", argv)
}

func TestSwitchProvider_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.usecase.SwitchProvider(ctx, "does-not-exist", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: chat")
}

func TestSwitchProvider_MissingActiveSegment_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// A chat with its only segment ended has no active segment to switch from.
	_, err := f.repo.Create(ctx, agentchat.CreateInput{
		ID: "c1", WorkspaceID: "ws1", SegmentID: "s1", CrowbarSegmentID: "cs1", ProviderID: "claude", TerminalSession: "term-x",
	})
	require.NoError(t, err)
	_, err = f.repo.EndSegment(ctx, "c1", "s1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.SwitchProvider(ctx, "c1", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: active segment")
}

func TestSwitchProvider_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.ws.err = errors.New("boom: worktree lookup")
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
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
