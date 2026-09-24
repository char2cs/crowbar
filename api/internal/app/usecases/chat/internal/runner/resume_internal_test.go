package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubChatsForProviderResolution answers the one read lastActiveProviderID
// makes. The embedded nil interface panics on anything else.
type stubChatsForProviderResolution struct {
	agentchat.EventStore
	chat domain.Chat
	err  error
}

func (s stubChatsForProviderResolution) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, s.err
}

// A chat read that BLEW UP is not the same fact as a chat with no provider
// recorded, and must never be flattened into one: the client's response to
// ErrChatProviderUnknown is to stop asking, which would make a transient
// failure look like permanent data loss.
func TestLastActiveProviderID_AChatReadFailurePropagates(t *testing.T) {
	boom := errors.New("projection down")
	rs := &Runners{chats: stubChatsForProviderResolution{err: boom}}

	_, err := rs.lastActiveProviderID(context.Background(), "c1")

	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrChatProviderUnknown)
}

func TestLastActiveProviderID_IsTheChatsOwnProvider(t *testing.T) {
	rs := &Runners{chats: stubChatsForProviderResolution{chat: domain.Chat{ID: "c1", ProviderID: "codex"}}}

	got, err := rs.lastActiveProviderID(context.Background(), "c1")

	require.NoError(t, err)
	assert.Equal(t, "codex", got)
}

func TestLastActiveProviderID_NoneRecordedIsUnknown(t *testing.T) {
	rs := &Runners{chats: stubChatsForProviderResolution{chat: domain.Chat{ID: "c1"}}}

	_, err := rs.lastActiveProviderID(context.Background(), "c1")

	require.ErrorIs(t, err, ErrChatProviderUnknown)
}
