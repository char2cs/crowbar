package chat_test

import (
	"testing"

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

// Busy is the ledger's fold as of the frame.
func TestSnapshot_BusyFollowsTheTurnAndVersionsOnlyGrow(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	prompt(t, f, runnerID, "claude", "think hard")
	busy, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.True(t, busy.Ledger.Busy)

	turn(t, f, runnerID, "claude", "done")
	idle, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.False(t, idle.Ledger.Busy)
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
	assert.False(t, last.Ledger.Busy)
}

// A switch parked on an unacknowledged prompt reports "switching" while it
// waits — the pane renders the phase, it does not infer it.
func TestSnapshot_AParkedSwitchReportsSwitching(t *testing.T) {
	f, chatID, carrier, done := switchParkedOnAPendingPrompt(t)

	snap, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, agentusecase.ChatPhaseSwitching, snap.Phase)

	require.NoError(t, f.usecase.IngestHook(f.ctx, carrier, "claude", "idle", mustJSON(t,
		map[string]any{"message": "Claude is waiting for your input"})))
	require.NoError(t, awaitSwitch(t, done).err)
	after, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, agentusecase.ChatPhaseLive, after.Phase, "the switch that went ahead leaves the chat live")
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
