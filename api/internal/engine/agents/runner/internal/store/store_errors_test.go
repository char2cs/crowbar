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
