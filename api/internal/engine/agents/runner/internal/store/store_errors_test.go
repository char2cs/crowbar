package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveRunnersForChat_StorageError(t *testing.T) {
	h := newHarness(t)
	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = h.st.LiveRunnersForChat(h.ctx, "chat-1")

	assert.Error(t, err)
}

func TestLiveRunnersForSession_StorageError(t *testing.T) {
	h := newHarness(t)
	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = h.st.LiveRunnersForSession(h.ctx, "ws-1", "session-1")

	assert.Error(t, err)
}

func TestConversationsForChat_StorageError(t *testing.T) {
	h := newHarness(t)
	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = h.st.ConversationsForChat(h.ctx, "chat-1")

	assert.Error(t, err)
}

func TestPlacementsForChat_StorageError(t *testing.T) {
	h := newHarness(t)
	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = h.st.PlacementsForChat(h.ctx, "chat-1")

	assert.Error(t, err)
}

// An empty chat id is NOWHERE, and nowhere has held nobody. It must answer an
// empty list rather than matching every row whose chat id happens to be blank —
// the same lie LiveRunnerForChat refuses to tell for the same reason.
func TestPlacementsForChat_EmptyChatIDIsNowhere(t *testing.T) {
	h := newHarness(t)

	placements, err := h.st.PlacementsForChat(h.ctx, "")

	require.NoError(t, err)
	assert.Empty(t, placements)
}

// ForgetChat's placement half fails LOUDLY, not quietly after its conversation
// half succeeded: a cascade that dropped a chat's conversations and kept its
// placements would leave a deleted chat still resolvable to a provider, which is
// the dangling-chat shape the whole delete cascade exists to prevent.
//
// The placement table alone is removed, so the conversation delete succeeds and
// only the second half can be what fails — closing the whole DB would fail the
// first one and prove nothing about this branch.
func TestForgetChat_PlacementStorageError(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.db.Exec("DROP TABLE agent_chat_placements").Error)

	err := h.st.ForgetChat(h.ctx, "chat-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "forget placements for chat")
}
