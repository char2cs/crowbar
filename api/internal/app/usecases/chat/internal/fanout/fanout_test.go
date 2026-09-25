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
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

type emptyReader struct{}

func (emptyReader) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id}, nil
}
func (emptyReader) AllLive(context.Context) ([]agents.Runner, error) { return nil, nil }

func owner(t *testing.T) (*snapshot.Snapshots, *[]snapshot.Frame) {
	t.Helper()
	s := snapshot.New(1)
	var frames []snapshot.Frame
	s.SetPublish(func(f snapshot.Frame) { frames = append(frames, f) })
	s.Bind(emptyReader{}, nil)
	return s, &frames
}

// A chat event reaches the owner with the aggregate it carried: the frame's
// working is the command-side fold, not a re-read.
func TestFanout_ChatEventBecomesASnapshotFrame(t *testing.T) {
	s, frames := owner(t)
	fanout.New(s).ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-1", WorkspaceID: "ws-1", Kind: "turn_started", Working: true,
		Chat: domain.Chat{WorkspaceID: "ws-1", Working: true}, Version: 3,
	})

	require.Len(t, *frames, 1)
	f := (*frames)[0]
	assert.Equal(t, "turn_started", f.Kind)
	assert.Equal(t, "chat-1", f.Snapshot.Chat.ID)
	assert.True(t, f.Snapshot.Chat.Working)
}

func TestFanout_ForgottenChatIsADeleteFrame(t *testing.T) {
	s, frames := owner(t)
	fanout.New(s).ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-9", WorkspaceID: "ws-1", Kind: "deleted", Working: true, Forgotten: true,
	})

	require.Len(t, *frames, 1)
	assert.True(t, (*frames)[0].Deleted)
	assert.Equal(t, "chat-9", (*frames)[0].Snapshot.Chat.ID)
}

func TestFanout_RunnerEventPlacesTheRunner(t *testing.T) {
	s, frames := owner(t)
	fanout.New(s).RunnerWatch()(agentrunner.RunnerEvent{
		RunnerID: "run-1", WorkspaceID: "ws-1", ChatID: "chat-1", Kind: "started",
		Runner: agents.Runner{CurrentChatID: "chat-1", ProviderID: "claude"}, Version: 1,
	})

	require.Len(t, *frames, 1)
	f := (*frames)[0]
	assert.Equal(t, "run-1", f.RunnerID)
	require.NotNil(t, f.Snapshot.Live)
	assert.Equal(t, "run-1", f.Snapshot.Live.ID)
}
