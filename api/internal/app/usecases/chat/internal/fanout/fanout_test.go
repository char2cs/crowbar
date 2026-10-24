package fanout_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/fanout"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

type emptyReader struct{}

func (emptyReader) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id}, nil
}
func (emptyReader) ViewLedger(_ context.Context, _ string, fn func(ledger.State) error) error {
	return fn(ledger.State{})
}

func owner(t *testing.T) (*snapshot.Snapshots, *[]snapshot.Frame) {
	t.Helper()
	s := snapshot.New(1)
	var frames []snapshot.Frame
	s.SetPublish(func(f snapshot.Frame) { frames = append(frames, f) })
	s.Bind(emptyReader{}, nil)
	return s, &frames
}

// A chat event reaches the owner with the aggregate it carried, not a re-read.
func TestFanout_ChatEventBecomesASnapshotFrame(t *testing.T) {
	s, frames := owner(t)
	fanout.New(s).ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-1", WorkspaceID: "ws-1", Kind: "title_set",
		Chat: domain.Chat{WorkspaceID: "ws-1", Title: "t"}, Version: 3,
	})

	require.Len(t, *frames, 1)
	f := (*frames)[0]
	assert.Equal(t, "title_set", f.Kind)
	assert.Equal(t, "chat-1", f.Snapshot.Chat.ID)
	assert.Equal(t, "t", f.Snapshot.Chat.Title)
}

func TestFanout_ForgottenChatIsADeleteFrame(t *testing.T) {
	s, frames := owner(t)
	fanout.New(s).ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-9", WorkspaceID: "ws-1", Kind: "deleted", Forgotten: true,
	})

	require.Len(t, *frames, 1)
	assert.True(t, (*frames)[0].Deleted)
	assert.Equal(t, "chat-9", (*frames)[0].Snapshot.Chat.ID)
}
