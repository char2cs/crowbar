package runner

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// spyStopTurns answers RecordStop by recording every chatID it was called
// with, so TestStopChat below can assert it fired without caring what the
// turn package does with the call — that behaviour has its own tests one
// package over (turn/stop_internal_test.go).
type spyStopTurns struct {
	noopTurns

	mu       sync.Mutex
	recorded []string
}

func (s *spyStopTurns) RecordStop(_ context.Context, chatID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded = append(s.recorded, chatID)
	return nil
}

// ChatWorking overrides noopTurns' own false: this test's whole scenario IS a
// turn in flight — the fake server on the other end only ever answers
// turn/interrupt, and StopChat now asks that only while one is genuinely
// running (see lifecycle.go's own "ONLY WHILE THERE IS A TURN TO INTERRUPT").
// Left at noopTurns' false, StopChat would skip interruptTurn entirely and
// fall to retire(), which this test's runnerStore stub cannot service.
func (s *spyStopTurns) ChatWorking(context.Context, string) (bool, error) { return true, nil }

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
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	spy := &spyStopTurns{}
	rs := &Runners{
		apiConns:    newAPIConnRegistry(),
		runnerStore: stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"}},
		ws:          stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:      stubAgentsForInterrupt{agent: agent},
		spawns:      inflight.NewGate(),
		turns:       spy,
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	done := make(chan error, 1)
	go func() { done <- rs.StopChat(ctx, "chat-1") }()

	// While the (fake) CLI's interrupt reply is still withheld — exactly the
	// state a real, still-generating codex sits in — StopChat must not yet
	// have recorded anything.
	time.Sleep(200 * time.Millisecond)
	spy.mu.Lock()
	recordedEarly := len(spy.recorded)
	spy.mu.Unlock()
	require.Zero(t, recordedEarly,
		"RecordStop fired before the CLI's interrupt actually resolved — this is the reported bug")

	close(release) // now let the (fake) app-server answer, as codex does once the turn truly ends

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("StopChat never returned once the interrupt resolved")
	}
	spy.mu.Lock()
	defer spy.mu.Unlock()
	require.Equal(t, []string{"chat-1"}, spy.recorded,
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
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

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
		turns:         stubTurnsForAttach{working: false}, // IDLE: no turn in flight
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
}
