package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// §7-A target 1, end to end through the real stores and the real fanout: the
// snapshot a frame carries and the one a read returns come from one owner,
// carry the command-side fold, and are ordered by version.

func TestSnapshot_ASpawnedChatIsLiveAndAFrameSaysSo(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	snap, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	require.NotNil(t, snap.Live)
	assert.Equal(t, runnerID, snap.Live.ID)
	assert.Equal(t, agentusecase.ChatPhaseLive, snap.Phase)

	frames := f.snapFrames.forChat(chatID)
	require.NotEmpty(t, frames, "the spawn was announced as a snapshot frame")
	last := frames[len(frames)-1].Snapshot
	assert.Equal(t, snap.Version, last.Version, "a read is the latest frame's answer, not a newer one")
	require.NotNil(t, last.Live)
	assert.Equal(t, runnerID, last.Live.ID)
}

// Working is the aggregate's own fold carried on the event — not the read
// model, which may not have caught up when the frame is built.
func TestSnapshot_WorkingFollowsTheTurnAndVersionsOnlyGrow(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	prompt(t, f, runnerID, "claude", "think hard")
	busy, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.True(t, busy.Chat.Working)

	turn(t, f, runnerID, "claude", "done")
	idle, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.False(t, idle.Chat.Working)
	assert.Greater(t, idle.Version, busy.Version)

	var last int64
	for _, fr := range f.snapFrames.forChat(chatID) {
		assert.Greater(t, fr.Snapshot.Version, last, "frames leave in version order")
		last = fr.Snapshot.Version
	}
}

// Stop leaves the chat dormant, and the last frame about it says so.
func TestSnapshot_StopEndsDormant(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	frames := f.snapFrames.forChat(chatID)
	require.NotEmpty(t, frames)
	sawStopping := false
	for _, fr := range frames {
		if fr.Snapshot.Phase == agentusecase.ChatPhaseStopping {
			sawStopping = true
		}
	}
	assert.True(t, sawStopping, "Stop announces its own phase")
	last := frames[len(frames)-1].Snapshot
	assert.Nil(t, last.Live)
	assert.Equal(t, agentusecase.ChatPhaseDormant, last.Phase)
	assert.False(t, last.Chat.Working)
}

// A switch parked on the outgoing turn reports "switching" while it waits —
// the pane renders the phase, it does not infer it.
func TestSnapshot_AParkedSwitchReportsSwitching(t *testing.T) {
	f := newFixture(t)
	agentusecase.SetSwitchAwaitTimeout(f.usecase.RunnerUsecase, time.Hour)
	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard")

	parked := parkedOnTurn(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
		done <- err
	}()
	<-parked

	snap, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, agentusecase.ChatPhaseSwitching, snap.Phase)

	cancel()
	<-done
	after, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, agentusecase.ChatPhaseLive, after.Phase, "an abandoned switch leaves the chat as it was")
	assert.Greater(t, after.Version, snap.Version)
}

// A7: a deleted chat leaves no snapshot entry behind.
func TestSnapshot_ADeletedChatIsForgotten(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	_, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	_, err = f.usecase.ChatSnapshot(f.ctx, chatID)
	require.Error(t, err)
	frames := f.snapFrames.forChat(chatID)
	require.NotEmpty(t, frames)
	assert.True(t, frames[len(frames)-1].Deleted)
}
