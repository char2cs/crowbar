package agent_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TestDeleteChat_TerminatesActiveSegmentPTY_AndSoftDeletes pins the standalone
// chat-delete half of Task 12 (the workspace-delete cascade's PTY teardown +
// Forget lives in repositories.Container.forgetAgentChats): DeleteChat
// terminates the chat's active segment's live vendor-CLI PTY BEFORE
// soft-deleting the chat (Status=deleted). The chat stays readable by direct
// GetChat (agentchat.EventStore.Delete's tombstone contract) but drops out of
// ListChats.
func TestDeleteChat_TerminatesActiveSegmentPTY_AndSoftDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	active := activeSegOf(t, f.chat(t, chatID), segID)
	require.NotEmpty(t, active.TerminalSessionID)

	require.NoError(t, f.usecase.DeleteChat(ctx, chatID))

	assert.Contains(t, f.term.terminatedIDs(), active.TerminalSessionID)

	chat := f.chat(t, chatID)
	assert.Equal(t, domain.AgentChatStatusDeleted, chat.Status)

	chats, err := f.usecase.ListChats(ctx)
	require.NoError(t, err)
	for _, c := range chats {
		assert.NotEqual(t, chatID, c.ID, "a deleted chat must not appear in ListChats")
	}
}

// TestDeleteChat_TerminateFailure_SessionAlreadyGone_ContinuesDelete mirrors
// SwitchProvider's own tolerance for a terminal session that is already gone
// (the one error the real terminal engine returns today): the delete must
// still proceed rather than get stuck because the CLI process had already
// exited on its own.
func TestDeleteChat_TerminateFailure_SessionAlreadyGone_ContinuesDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	require.NoError(t, f.usecase.DeleteChat(ctx, chatID))

	chat := f.chat(t, chatID)
	assert.Equal(t, domain.AgentChatStatusDeleted, chat.Status)
}

// TestDeleteChat_TerminateFailure_OtherError_IsBestEffort_StillDeletes: a
// genuine TerminateGraceful failure (not "session already gone") must NOT
// abort the delete. Terminate is best-effort — wedging the delete on a
// terminate error would trap the chat undeletable forever, a worse outcome
// than an orphaned PTY (reaped on the next daemon restart). The delete
// proceeds; the chat is soft-deleted.
func TestDeleteChat_TerminateFailure_OtherError_IsBestEffort_StillDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	active := activeSegOf(t, f.chat(t, chatID), segID)
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.NoError(t, f.usecase.DeleteChat(ctx, chatID), "a genuine terminate failure must not abort the delete (best-effort)")

	// The terminate WAS attempted (best-effort, not skipped) — terminateRequestIDs
	// records attempts even when they error, unlike terminatedIDs (success-only) ...
	assert.Contains(t, f.term.terminateRequestIDs(), active.TerminalSessionID)
	// ... and the delete proceeded despite the terminate error.
	assert.Equal(t, domain.AgentChatStatusDeleted, f.chat(t, chatID).Status)
}

// TestDeleteChat_NoActiveSegment_SkipsTerminate_StillDeletes: a chat whose
// active segment has already ended (ActiveSegmentID cleared) has nothing to
// terminate, but the delete must still proceed.
func TestDeleteChat_NoActiveSegment_SkipsTerminate_StillDeletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.EndSegment(ctx, chatID, segID, timeUnix(2))
	require.NoError(t, err)
	f.wait()
	require.Empty(t, f.chat(t, chatID).ActiveSegmentID, "the segment must be ended before this test's assertion is meaningful")

	require.NoError(t, f.usecase.DeleteChat(ctx, chatID))

	assert.Empty(t, f.term.terminatedIDs(), "no active segment means nothing to terminate")
	assert.Equal(t, domain.AgentChatStatusDeleted, f.chat(t, chatID).Status)
}

// TestDeleteChat_UnknownChat_ReturnsWrappedError: DeleteChat on an id with no
// chat wraps the GetChat lookup failure rather than panicking or silently
// no-oping.
func TestDeleteChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.DeleteChat(ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete chat: get")
}
