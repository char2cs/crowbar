package runner

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// A connection that dies on its own ends its runner (the channel IS the
// runner) with the loss recorded as the cause — never a runner left placed on
// its chat with no channel at all.
func TestRegression_ALostAPIConnectionEndsItsRunnerWithACause(t *testing.T) {
	// Hang up as soon as the handshake is done: the driver's Events() channel
	// closes, which is exactly what a dead `serve` process looks like from here.
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) { _ = conn.Close() })

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)

	rs := &Runners{turns: &spyTurns{}, apiConns: newAPIConnRegistry(), sessions: newSessionBook()}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.apiConns.set("runner-1", conn)
	require.True(t, rs.HasLiveAPIConnection("runner-1"))

	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return !rs.HasLiveAPIConnection("runner-1") },
		3*time.Second, 10*time.Millisecond, "a dead connection must stop claiming to be the runner's channel")
	require.Equal(t, domain.AgentExitConnectionLost, rs.sessions.takeCause("runner-1"))
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

	rs := &Runners{turns: &spyTurns{}, apiConns: newAPIConnRegistry()}
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
