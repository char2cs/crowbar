package conversation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Every chat is minted typed, so the backfill has nothing to write and the
// listing of what it would leave behind is empty.
func TestBackfillChatTypes_AMintedChatNeedsNoBackfill(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	id, err := f.conversations.MintChat(t.Context(), "", "ws-1", "vendor-a", "")
	require.NoError(t, err)
	f.settle()

	skipped, err := f.conversations.BackfillChatTypes(t.Context())
	require.NoError(t, err)
	assert.Zero(t, skipped)
	untyped, err := f.conversations.UntypedChats(t.Context())
	require.NoError(t, err)
	assert.Empty(t, untyped)
	chat, err := f.chats.LoadChat(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, domain.ChatTypeChat, chat.Type)
}
