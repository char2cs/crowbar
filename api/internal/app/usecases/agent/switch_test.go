package agent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func TestSwitchProvider_KillsOutgoingTerminal_AndEndsOldSegment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	oldSeg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	require.NotEmpty(t, oldSeg.TerminalSessionID)

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	assert.Contains(t, f.term.killed, oldSeg.TerminalSessionID)

	ended, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "ended", ended.Status)
	require.NotNil(t, ended.EndedAt)
}

// TestSwitchProvider_KillFailure_SessionAlreadyGone_ContinuesSwitch guards the
// fix that surfaces (rather than swallows) Kill's error: when Kill fails
// because the terminal session is already gone (the one error the real
// terminal engine's Kill can return today), the switch must still proceed —
// aborting would trap a chat that could never switch again once its terminal
// session ends on its own.
func TestSwitchProvider_KillFailure_SessionAlreadyGone_ContinuesSwitch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	f.term.killErr = fmt.Errorf("terminal: kill: %w: term-1", engineterminal.ErrSessionNotFound)

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newSegID)

	oldSeg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "ended", oldSeg.Status)

	chat, err := f.repo.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, newSegID, chat.ActiveSegmentID)
}

// TestSwitchProvider_KillFailure_OtherError_AbortsSwitch guards the other half
// of the same fix: a Kill failure that is NOT "session already gone" must
// abort the switch entirely rather than proceed to spawn a second live CLI
// into the same worktree while the DB marks only the new one active.
func TestSwitchProvider_KillFailure_OtherError_AbortsSwitch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	f.term.killErr = errors.New("boom: kill genuinely failed")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kill outgoing terminal")

	oldSeg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "active", oldSeg.Status, "old segment must NOT be marked ended when Kill genuinely failed")

	require.Len(t, f.term.calls, 1, "no new segment/terminal should have been spawned after a real kill failure")

	chat, err := f.repo.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, segID, chat.ActiveSegmentID, "active segment must be unchanged")
}

// TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeKill guards the fix
// that surfaces (rather than swallows) AssembleHandoff's error: previously the
// switch proceeded with a silently EMPTY handoff. Since AssembleHandoff runs
// BEFORE Kill, aborting here must leave the chat completely untouched.
func TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeKill(t *testing.T) {
	// AssembleHandoff makes its own internal GetChat(chatID) call; SwitchProvider's
	// own leading GetChat is call #1, so #2 is AssembleHandoff's.
	repo := &erroringStore{Store: newRealStore(t), failGetChatAt: 2}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assemble handoff")

	assert.Empty(t, f.term.killed, "the outgoing terminal must never be killed when handoff assembly fails first")
	seg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "active", seg.Status, "old segment must be untouched")
	require.Len(t, f.term.calls, 1, "no new segment/terminal should have been spawned")
}

func TestSwitchProvider_Forward_SpawnsTargetProviderWithHandoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	appendTranscript(t, f, segID, "sid-1", "prior turn content for handoff")

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newSegID)

	require.Len(t, f.term.calls, 2)
	newCall := f.term.calls[1]
	assert.Equal(t, "codex", newCall.argv[0])
	assert.Contains(t, strings.Join(newCall.argv, "\x00"), "prior turn content for handoff")
}

func TestSwitchProvider_PersistsNewActiveSegmentForTargetProvider(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	newSeg, err := f.repo.GetSegment(ctx, newSegID)
	require.NoError(t, err)
	assert.Equal(t, "codex", newSeg.ProviderID)
	assert.Equal(t, "active", newSeg.Status)
	assert.NotEmpty(t, newSeg.TerminalSessionID)

	chat, err := f.repo.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, newSegID, chat.ActiveSegmentID)
}

func TestSwitchProvider_Broadcasts_Switched(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.bc.calls = nil

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	require.Len(t, f.bc.calls, 1)
	assert.Equal(t, chatID, f.bc.calls[0].chatID)
	assert.Equal(t, "switched", f.bc.calls[0].kind)
}

// TestSwitchProvider_SwitchBack_ResumesNativeSessionWithSeparateArgvTokens
// drives a full forward+back sequence: spawn provider "claude", let it bind a
// native ProviderSessionID via session_start, switch forward to "codex", then
// switch back to "claude". The switch-back must resume the prior claude
// session by expanding+tokenizing descriptor.Session.Resume.Arg ("--resume
// {id}") into two SEPARATE argv elements, not one "--resume <id>" string.
func TestSwitchProvider_SwitchBack_ResumesNativeSessionWithSeparateArgvTokens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{
		"session_id":      "sid-claude-native",
		"transcript_path": "/tmp/whatever.jsonl",
	}))

	boundSeg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	require.Equal(t, "sid-claude-native", boundSeg.ProviderSessionID)

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	require.Len(t, f.term.calls, 2)

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newSeg, err := f.repo.GetSegment(ctx, newSegID)
	require.NoError(t, err)
	assert.Equal(t, "claude", newSeg.ProviderID)

	require.Len(t, f.term.calls, 3)
	argv := f.term.calls[2].argv

	resumeIdx := indexOf(argv, "--resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain --resume as its own token", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-claude-native", argv[resumeIdx+1])

	// Must be two distinct tokens, not one combined "--resume sid-..." token.
	assert.NotContains(t, argv, "--resume sid-claude-native")
}

// TestSwitchProvider_SwitchBack_ResumeStepsPrecedeHandoff exercises the
// codex-target switch-back path, where the resume arg is the subcommand-style
// "resume {id}" (no leading dash) and MUST precede the positional handoff arg.
func TestSwitchProvider_SwitchBack_ResumeStepsPrecedeHandoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "codex")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{
		"session_id":      "sid-codex-native",
		"transcript_path": "/tmp/whatever.jsonl",
	}))

	appendTranscript(t, f, segID, "sid-codex-native", "codex ledger content")

	_, err = f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	newSeg, err := f.repo.GetSegment(ctx, newSegID)
	require.NoError(t, err)
	assert.Equal(t, "codex", newSeg.ProviderID)

	require.Len(t, f.term.calls, 3)
	argv := f.term.calls[2].argv

	resumeIdx := indexOf(argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain resume as its own token", argv)
	require.Less(t, resumeIdx+1, len(argv))
	assert.Equal(t, "sid-codex-native", argv[resumeIdx+1])

	// Codex's handoff_inject is a bare positional; it must come after the
	// resume subcommand+id pair.
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

	require.NoError(t, f.repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "ws1"}))

	_, err := f.usecase.SwitchProvider(ctx, "c1", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: active segment")
}

func TestSwitchProvider_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

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

	_, err = f.usecase.SwitchProvider(ctx, chatID, "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
}

func TestSwitchProvider_ListSegmentsFailure_ReturnsWrappedError(t *testing.T) {
	repo := &listFailingStore{Store: newRealStore(t)}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	repo.fail = true
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list segments")
}

func TestSwitchProvider_OldSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t)}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	// SpawnChat makes 2 SaveSegment calls (create + stamp terminal session
	// id); SwitchProvider's "mark ended" save is the 3rd.
	repo.failSaveSegAt = 3
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: save old segment")
}

// listFailingStore wraps a real Store and lets a test force
// ListSegmentsByChat to fail once armed, exercising SwitchProvider's guard
// clause around it.
type listFailingStore struct {
	agentchat.Store
	fail bool
}

func (s *listFailingStore) ListSegmentsByChat(ctx context.Context, chatID string) ([]domain.AgentSegment, error) {
	if s.fail {
		return nil, errors.New("boom: list segments")
	}
	return s.Store.ListSegmentsByChat(ctx, chatID)
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}
