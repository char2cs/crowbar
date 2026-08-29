package runner

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newTestRunnersWithPrompts is a minimal Runners fixture for exercising the
// prompt-journal helpers directly, without the full chat usecase harness.
func newTestRunnersWithPrompts(home string) *Runners {
	return &Runners{
		home:    func() (string, error) { return home, nil },
		prompts: agentjournal.NewPromptRequests(),
	}
}

// TestPromptJournalDirFor_StableAcrossPromotion pins spec §1.5's invariant: a
// chat's ledger location must be a pure function of its own id, never of its
// WorkspaceID. WorkspaceID is optional and mutable (a bubble chat has none
// until promoted), so a chat with no workspace and the SAME chat once promoted
// into one must resolve to the identical journal directory.
//
// Before this fix, the directory came from rs.ws.AgentChatsDir(ctx,
// chat.WorkspaceID), which resolves a workspace row by id — an empty
// WorkspaceID has no such row, so the lookup errored outright rather than
// merely disagreeing with the promoted path. promptJournalDirFor now takes no
// workspace input at all, so nothing about a chat's WorkspaceID can move it.
func TestPromptJournalDirFor_StableAcrossPromotion(t *testing.T) {
	rs := newTestRunnersWithPrompts(t.TempDir())

	before := domain.Chat{ID: "chat-1", WorkspaceID: ""}
	after := domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}

	beforeDir, err := rs.promptJournalDirFor(before.ID)
	require.NoError(t, err, "a workspace-less chat must resolve a journal dir, not error")
	afterDir, err := rs.promptJournalDirFor(after.ID)
	require.NoError(t, err)

	assert.Equal(t, beforeDir, afterDir,
		"ledger path must be a function of chat id, not workspace")
}

// TestPromptJournalDirFor_KeyedByChatID guards the other half of the same
// invariant: two DIFFERENT chats must never collide on one journal directory.
func TestPromptJournalDirFor_KeyedByChatID(t *testing.T) {
	home := t.TempDir()
	rs := newTestRunnersWithPrompts(home)

	dirA, err := rs.promptJournalDirFor("chat-a")
	require.NoError(t, err)
	dirB, err := rs.promptJournalDirFor("chat-b")
	require.NoError(t, err)

	assert.NotEqual(t, dirA, dirB)
	assert.Equal(t, filepath.Join(home, "chats", "chat-a", "prompt-requests"), dirA)
}

// TestPromptJournalDirFor_PropagatesHomeFailure: the derivation is best-effort
// on crowbar home, exactly like every other path-deriving call in this
// package — a resolver failure must surface as an error, not a panic or a
// silently wrong path.
func TestPromptJournalDirFor_PropagatesHomeFailure(t *testing.T) {
	boom := errors.New("boom: crowbar home")
	rs := &Runners{home: func() (string, error) { return "", boom }}

	_, err := rs.promptJournalDirFor("chat-1")
	require.ErrorIs(t, err, boom)
}
