package runner

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// pumpAPIConn's loop used to just RETURN when the driver's Events() channel
// closed — no teardown of any kind. The connection was dead but its registry
// entry stayed, so HasLiveAPIConnection went on answering true forever, and
// apiOwnsThisEvent (turn/ingest.go) kept DROPPING the companion PTY's hooks copy
// of every api-owned event as a redundant duplicate of an api transport that no
// longer existed. The chat went permanently silent with its spinner stuck on,
// and nothing else could reach it: the companion PTY is still alive, so no
// runner-exit reconcile fires, and neither termwait sweep applies to a clean
// screen that streamed nothing.
func TestRegression_ALostAPIConnectionStopsClaimingToBeLive(t *testing.T) {
	// Hang up as soon as the handshake is done: the driver's Events() channel
	// closes, which is exactly what a dead `serve` process looks like from here.
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) { _ = conn.Close() })

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)

	rs := &Runners{turns: &spyTurns{}, apiConns: newAPIConnRegistry()}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.apiConns.set("runner-1", conn)
	require.True(t, rs.HasLiveAPIConnection("runner-1"))

	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return !rs.HasLiveAPIConnection("runner-1") },
		3*time.Second, 10*time.Millisecond,
		"a dead connection still reporting live suppresses the hooks fallback forever")
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
	apiConn, err := agent.StartAPIConn(context.Background(), sockPath)
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
