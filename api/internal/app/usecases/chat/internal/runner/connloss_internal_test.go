package runner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// A connection that dies on its own is dropped, keeping why on itself: it names
// the runner's exit only when it was that runner's watched process.
func TestOnAPIConnLost_KeepsWhyOnTheConnectionAndDropsIt(t *testing.T) {
	// Hang up as soon as the handshake is done: the driver's Events() channel
	// closes, which is exactly what a dead `serve` process looks like from here.
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) { _ = conn.Close() })

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)

	rs := &Runners{turns: &spyTurns{}, apiConns: newAPIConnRegistry(), live: newLiveTable()}
	require.NoError(t, rs.live.add(engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"}))
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.apiConns.set("runner-1", conn)
	require.True(t, rs.HasLiveAPIConnection("runner-1"))

	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return !rs.HasLiveAPIConnection("runner-1") },
		3*time.Second, 10*time.Millisecond, "a dead connection must stop claiming to be the runner's channel")
	require.Equal(t, domain.AgentExitConnectionLost, conn.lostReason())
	require.Empty(t, exitCause(t, rs.live, "runner-1"), "nothing watched it as the runner's process")
}

// The mirror case: a DELIBERATE teardown (drop, on retire / provider switch /
// PTY exit / SwitchToTerminal) cancels the connection's own ctx, and the caller
// that dropped it already owns closing the turn. The pump must not treat that as
// a lost connection and reconcile a second time — nor panic re-dropping a runner
// the registry has already forgotten.
func TestPumpAPIConn_ADeliberateTeardownIsNotTreatedAsALostConnection(t *testing.T) {
	released := make(chan struct{})
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		<-released
		_ = conn.Close()
	})

	agent := apiTransportTestAgent(t)
	apiConn, err := agent.StartAPIConn(context.Background(), sockPath, nil)
	require.NoError(t, err)

	rs := &Runners{turns: &spyTurns{}, apiConns: newAPIConnRegistry(), live: newLiveTable()}
	connCtx, connCancel := context.WithCancel(context.Background())
	conn := &apiconn{driver: apiConn, ctx: connCtx, cancel: connCancel}
	rs.apiConns.set("runner-1", conn)
	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	// drop() is the deliberate path: it cancels conn.ctx, closes the driver and
	// forgets the entry, all before the pump notices Events() closing.
	rs.apiConns.drop("runner-1")
	close(released)

	require.Never(t, func() bool { return rs.HasLiveAPIConnection("runner-1") },
		500*time.Millisecond, 25*time.Millisecond)
}

// afterLoss blocks until runnerID's api connection is gone, so a test puts the
// loss before whatever step of the spawn calls it.
type afterLoss struct {
	conns    *apiConnRegistry
	runnerID string
}

func (a afterLoss) wait(ctx context.Context) error {
	c, ok := a.conns.get(a.runnerID)
	if !ok {
		return nil
	}
	select {
	case <-c.ctx.Done():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("the api connection was never lost: %w", ctx.Err())
	}
}

// forkAfterLoss is a terminal that forks only once the connection is lost.
type forkAfterLoss struct {
	*fakeTermForAttach
	afterLoss
}

func (f forkAfterLoss) CreateCommand(
	ctx context.Context, chatID, cwd string, argv, env []string, onExit func(engineterminal.Exit),
) (string, error) {
	if err := f.wait(ctx); err != nil {
		return "", err
	}
	return f.fakeTermForAttach.CreateCommand(ctx, chatID, cwd, argv, env, onExit)
}

// createAfterLoss mints the chat only once the connection is lost: before the
// runner_started row, so the loss lands during startup.
type createAfterLoss struct {
	stubChatsForSpawn
	afterLoss
}

func (c createAfterLoss) Create(ctx context.Context, _ agentchat.CreateInput) (domain.Chat, error) {
	return c.chat, c.wait(ctx)
}

type seedingConversations struct{ stubConversationsForSpawn }

func (seedingConversations) SeedPermissionLevel(context.Context, string) {}

// A connection lost before its spawn forked the PTY is not why that runner
// ends: it ends as the user's Stop.
func TestSpawnRunner_AConnectionLostBeforeThePTYForkIsNoExitCause(t *testing.T) {
	h := helperServeSpawn(t, "drop-after thread/resume", true, "runner-1")
	h.rs.term = forkAfterLoss{h.term, afterLoss{h.rs.apiConns, "runner-1"}}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test", spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)
	require.Len(t, h.term.created, 1, "the spawn forked its PTY")
	require.False(t, h.rs.HasLiveAPIConnection("runner-1"), "the connection was lost first")

	require.NoError(t, h.rs.StopChat(ctx, h.chatID))
	assert.Equal(t, domain.AgentExitStopped, endedAs(t, h))
}

// An adopted connection is its runner's process: lost during startup, it still
// ends that runner as connection_lost.
func TestSpawnRunner_AnAdoptedConnectionLostDuringStartupEndsItsRunner(t *testing.T) {
	h := helperServeSpawn(t, "drop-after turn/start", false, "runner-1")
	h.rs.chats = createAfterLoss{stubChatsForSpawn{chat: domain.Chat{ID: h.chatID}}, afterLoss{h.rs.apiConns, "runner-1"}}
	h.rs.conversations = seedingConversations{}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1", promptMessage: "hi"}, true)
	if err != nil {
		require.ErrorIs(t, err, ErrProviderExitedDuringStartup)
	}
	require.Empty(t, h.term.created, "the connection was the runner")
	if v, ok := h.rs.live.view(h.chatID); ok {
		select {
		case <-v.exited:
		case <-ctx.Done():
			t.Fatal("the runner outlived its connection")
		}
	}
	assert.Equal(t, domain.AgentExitConnectionLost, endedAs(t, h))
}
