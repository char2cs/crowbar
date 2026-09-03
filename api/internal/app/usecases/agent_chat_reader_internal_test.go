package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeChatGetter struct {
	chat  domain.Chat
	chats []domain.Chat
	err   error
}

func (f fakeChatGetter) GetChat(
	context.Context,
	string,
) (domain.Chat, error) {
	if f.err != nil {
		return domain.Chat{}, f.err
	}
	return f.chat, nil
}

func (f fakeChatGetter) ListChats(
	context.Context,
) ([]domain.Chat, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.chats, nil
}

// TestAgentChatReader_Get_DelegatesToTheRepositorysGetChat proves the name
// translation this adapter exists for: the tool surface's plain Get calls the
// repository's GetChat (named GetChat because it also serves runners).
func TestAgentChatReader_Get_DelegatesToTheRepositorysGetChat(t *testing.T) {
	r := agentChatReader{chats: fakeChatGetter{chat: domain.Chat{ID: "c1", Title: "spikes"}}}

	got, err := r.Get(context.Background(), "c1")

	require.NoError(t, err)
	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "spikes", got.Title)
}

func TestAgentChatReader_Get_PropagatesTheUnderlyingError(t *testing.T) {
	wantErr := errors.New("chat store unavailable")
	r := agentChatReader{chats: fakeChatGetter{err: wantErr}}

	_, err := r.Get(context.Background(), "c1")

	assert.ErrorIs(t, err, wantErr)
}

func TestAgentChatReader_ListChats_DelegatesToTheRepository(t *testing.T) {
	r := agentChatReader{chats: fakeChatGetter{chats: []domain.Chat{{ID: "c1"}, {ID: "c2"}}}}

	got, err := r.ListChats(context.Background())

	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestAgentChatReader_ListChats_PropagatesTheUnderlyingError(t *testing.T) {
	wantErr := errors.New("chat store unavailable")
	r := agentChatReader{chats: fakeChatGetter{err: wantErr}}

	_, err := r.ListChats(context.Background())

	assert.ErrorIs(t, err, wantErr)
}
