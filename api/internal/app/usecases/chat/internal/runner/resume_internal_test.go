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

type stubChatsForProviderResolution struct {
	agentchat.EventStore
	chat domain.Chat
	err  error
}

func (s stubChatsForProviderResolution) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, s.err
}

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
