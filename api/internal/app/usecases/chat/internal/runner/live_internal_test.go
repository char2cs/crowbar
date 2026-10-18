package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// liveWith is a live table holding runners, each started on its chat.
func liveWith(t *testing.T, runners ...engineagents.Runner) *liveTable {
	t.Helper()
	live := newLiveTable()
	for _, r := range runners {
		require.NoError(t, live.add(r))
		live.markStarted(r.ID)
	}
	return live
}

// exitCause is the cause noted for runnerID's exit; "" when none was.
func exitCause(t *testing.T, live *liveTable, runnerID string) string {
	t.Helper()
	exit, ok := live.claimExit(runnerID)
	require.True(t, ok, "runner %s is in the table", runnerID)
	return exit.cause
}

func TestLiveTable_OneEntryPerChat(t *testing.T) {
	live := liveWith(t, engineagents.Runner{ID: "r1", CurrentChatID: "chat-1"})

	err := live.add(engineagents.Runner{ID: "r2", CurrentChatID: "chat-1"})
	require.ErrorIs(t, err, ErrChatHasLiveRunner)
	require.NoError(t, live.add(engineagents.Runner{ID: "r3", CurrentChatID: "chat-2"}), "another chat is free")

	live.remove("r1")
	require.NoError(t, live.add(engineagents.Runner{ID: "r2", CurrentChatID: "chat-1"}), "a removed runner frees its chat")
}

func TestLiveTable_AnExitingRunnerStillResolvesUntilItsExit(t *testing.T) {
	rs := &Runners{live: liveWith(t, engineagents.Runner{ID: "r1", CurrentChatID: "chat-1"})}
	exited, ok := rs.live.markExiting("r1", "stopped")
	require.True(t, ok)
	rs.live.markExiting("r1", "displaced")

	_, err := rs.LiveRunnerForChat(t.Context(), "chat-1")
	require.NoError(t, err, "a hook of an exiting runner still finds its chat")
	_, ok = rs.RunnerByID("r1")
	assert.True(t, ok)
	_, onSnapshot := rs.LiveRunner("chat-1")
	assert.True(t, onSnapshot, "the snapshot shows it until its exit is recorded")

	assert.Equal(t, "stopped", exitCause(t, rs.live, "r1"), "the first cause noted wins")
	_, onSnapshot = rs.LiveRunner("chat-1")
	assert.False(t, onSnapshot, "an exit being recorded is off the snapshot")
	select {
	case <-exited:
		t.Fatal("the chat is not free until the runner is removed")
	default:
	}

	rs.live.remove("r1")
	<-exited
	_, err = rs.LiveRunnerForChat(t.Context(), "chat-1")
	require.ErrorIs(t, err, ErrNoLiveRunner)
	_, ok = rs.RunnerByID("r1")
	assert.False(t, ok)
}

func TestLiveTable_AnUnstartedSpawnIsShownToNoReader(t *testing.T) {
	rs := &Runners{live: newLiveTable()}
	require.NoError(t, rs.live.add(engineagents.Runner{ID: "r1", CurrentChatID: "chat-1"}))

	all, err := rs.AllLive(t.Context())
	require.NoError(t, err)
	assert.Empty(t, all)
	_, ok := rs.LiveRunner("chat-1")
	assert.False(t, ok)

	rs.live.markStarted("r1")
	all, err = rs.AllLive(t.Context())
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestAwaitDormant_WaitsOutAnExitingRunnerOnly(t *testing.T) {
	rs := &Runners{live: liveWith(t, engineagents.Runner{ID: "r1", CurrentChatID: "chat-1"})}

	r, live, err := rs.awaitDormant(t.Context(), "chat-1")
	require.NoError(t, err)
	require.True(t, live)
	assert.Equal(t, "r1", r.ID, "a runner that is not exiting is returned at once")

	rs.live.markExiting("r1", "stopped")
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = rs.awaitDormant(cancelled, "chat-1")
	require.ErrorIs(t, err, context.Canceled, "an exiting runner is waited out, never returned")

	rs.live.remove("r1")
	_, live, err = rs.awaitDormant(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.False(t, live, "the chat is dormant once the exiting runner is gone")
}
