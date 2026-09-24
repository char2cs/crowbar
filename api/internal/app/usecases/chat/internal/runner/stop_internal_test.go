package runner

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stopActivity records the stopped interruptions RecordStop writes — the only
// part of the activity ledger a Stop touches — and panics on anything else.
type stopActivity struct {
	agentactivity.EventStore

	mu      sync.Mutex
	stopped []string
}

func (a *stopActivity) Interrupt(_ context.Context, chatID, _, kind, _ string, _ time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if kind == engineagents.InterruptStopped {
		a.stopped = append(a.stopped, chatID)
	}
	return nil
}

func (a *stopActivity) ResolveInterruption(context.Context, string, string, string, string, time.Time) error {
	return nil
}

func (a *stopActivity) recorded() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.stopped...)
}

// realStopTurns is the REAL hook ingress over a recording ledger, sharing the
// in-flight registries with the Runners under test — so whether RecordStop
// fires, and how often, is decided by production code on both sides. A spy
// that recorded unconditionally is what hid P0-8: the real RecordStop used to
// re-ask "is a turn open" after the teardown had already closed it.
func realStopTurns(inflightTurns *inflight.Turns, work *inflight.Work) (*turn.Turns, *stopActivity) {
	activity := &stopActivity{}
	return turn.New(turn.Deps{
		Activity:      activity,
		InflightTurns: inflightTurns,
		Work:          work,
	}), activity
}

// TestRegression_StopChatRecordsTheStopOnlyAfterTheCLIActuallyStops guards
// the bug reported live 2026-09-09: the user clicked Stop mid-generation, and
// not only did the "Interrupted" divider land in the wrong place in the
// transcript, the underlying codex process kept generating — more tool calls,
// more assistant text — for a full extra minute afterward.
//
// Root cause: StopChat used to call RecordStop BEFORE attempting
// interruptTurn, durably marking the turn "Interrupted" the INSTANT Stop was
// clicked, unconditionally. But codex's own turn/interrupt genuinely defers
// its JSON-RPC reply until the turn actually ends (confirmed against
// codex-rs's turn_interrupt_inner/respond_to_pending_interrupts: a real
// interrupt returns Ok(None) — no reply at all — until TurnAborted or
// TurnComplete fires) — so interruptTurn's own Send, which blocks for that
// reply, does not mean the turn has stopped merely because it was called; it
// means the turn has stopped once it RETURNS. Recording before that point was
// both a lie (nothing had stopped yet) and out of order (real content kept
// landing in the ledger after the premature marker). The two symptoms are one
// bug, not two.
//
// This reuses interrupt_internal_test.go's own live-api-connection fixture so
// it stays end to end through StopChat itself, not a direct call to
// interruptTurn — the ordering under test is StopChat's — but withholds the
// fake server's reply exactly as codex's real app-server does, to prove
// RecordStop cannot fire until that reply (i.e., the actual stop) arrives.
func TestRegression_StopChatRecordsTheStopOnlyAfterTheCLIActuallyStops(t *testing.T) {
	release := make(chan struct{})
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // turn/interrupt
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		<-release // withheld, exactly like codex's own deferred turn/interrupt reply
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	agent := interruptTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	inflightTurns, work := inflight.NewTurns(), inflight.NewWork()
	inflightTurns.Begin("runner-1", "chat-1")
	work.Set("chat-1", true)
	turns, activity := realStopTurns(inflightTurns, work)
	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		runnerStore:   stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"}},
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		spawns:        inflight.NewGate(),
		inflightTurns: inflightTurns,
		turns:         turns,
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	done := make(chan error, 1)
	go func() { done <- rs.StopChat(ctx, "chat-1") }()

	// While the (fake) CLI's interrupt reply is still withheld — exactly the
	// state a real, still-generating codex sits in — StopChat must not yet
	// have recorded anything.
	time.Sleep(200 * time.Millisecond)
	require.Empty(t, activity.recorded(),
		"RecordStop fired before the CLI's interrupt actually resolved — this is the reported bug")

	close(release) // now let the (fake) app-server answer, as codex does once the turn truly ends

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("StopChat never returned once the interrupt resolved")
	}
	require.Equal(t, []string{"chat-1"}, activity.recorded(),
		"StopChat must record the interruption once the CLI has actually stopped")
}

// stopRetireRunnerStore answers LiveRunnerForChat with a fixed runner and
// records whether Displace was ever called — the one call retire() makes that
// TestRegression_StopChatOnAnIdleChatActuallyRetires needs to observe.
// runner.CurrentChatID is deliberately "" so displace()'s own
// reconcilePromptRunnerDeparture/closeAbandonedTurn calls both short-circuit
// on their own empty-chatID guard, needing no chats/activity/prompts stubs.
type stopRetireRunnerStore struct {
	agentrunner.EventStore
	runner    engineagents.Runner
	displaced bool
}

func (s *stopRetireRunnerStore) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	return s.runner, nil
}

func (s *stopRetireRunnerStore) Displace(
	context.Context, string,
) (engineagents.Runner, error) {
	s.displaced = true
	return s.runner, nil
}

// TestRegression_StopChatOnAnIdleChatActuallyRetires guards the bug reported
// live 2026-09-08 ("chats losing its provider when closing"): interruptTurn's
// own check has no notion of "idle" — a live api connection plus a descriptor
// that declares a non-"prompt" interrupt gesture (codex, always) made it
// return true regardless of whether a turn was actually running, so StopChat
// on an IDLE chat neither interrupted anything nor retired the runner.
// Confirmed live: closing a codex tab left its runner, its api connection and
// its companion PTY all running indefinitely, still placed on the chat the
// user had just "closed" — directly contradicting the frontend's own
// documented "closing stops the CLI" contract.
func TestRegression_StopChatOnAnIdleChatActuallyRetires(t *testing.T) {
	interrupted := make(chan struct{}, 1)
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err == nil {
			interrupted <- struct{}{}
		}
	})

	agent := interruptTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	idleTurns, activity := realStopTurns(inflight.NewTurns(), idleWork())
	store := &stopRetireRunnerStore{
		runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"},
	}
	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		attached:      newAttachRegistry(),
		runnerStore:   store,
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		spawns:        inflight.NewGate(),
		inflightTurns: inflight.NewTurns(),
		turns:         idleTurns,
		term:          &fakeTermForAttach{},
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	require.NoError(t, rs.StopChat(ctx, "chat-1"))

	select {
	case <-interrupted:
		t.Fatal("StopChat sent turn/interrupt on an idle chat — there was nothing running to interrupt")
	case <-time.After(200 * time.Millisecond):
	}
	require.True(t, store.displaced,
		"an idle chat's runner must actually be retired on close, not left running forever")
	require.Empty(t, activity.recorded(), "closing an idle chat interrupts nothing")
}

func idleWork() *inflight.Work {
	w := inflight.NewWork()
	w.Set("chat-1", false)
	return w
}

// P0-8, the interrupt path: codex answers turn/interrupt only once the turn
// has ended, and its turn_stop can be ingested BEFORE that reply reaches
// StopChat — completing the in-flight turn and clearing Working. The Stop
// still interrupted a running turn, so it must still leave exactly one
// divider.
func TestRegression_StopChatRecordsTheStopWhenTheTurnStopWinsTheRace(t *testing.T) {
	inflightTurns, work := inflight.NewTurns(), inflight.NewWork()
	inflightTurns.Begin("runner-1", "chat-1")
	work.Set("chat-1", true)
	turns, activity := realStopTurns(inflightTurns, work)

	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // turn/interrupt
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		// The turn_stop hook lands first, exactly as closeTurnFromStop does it.
		work.Set("chat-1", false)
		inflightTurns.Complete("runner-1")
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage()
	})

	agent := interruptTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		runnerStore:   stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"}},
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		spawns:        inflight.NewGate(),
		inflightTurns: inflightTurns,
		turns:         turns,
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	require.NoError(t, rs.StopChat(ctx, "chat-1"))
	require.Equal(t, []string{"chat-1"}, activity.recorded())
}
