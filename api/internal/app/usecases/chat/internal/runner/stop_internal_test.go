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

// TestStopChat_RecordsTheStopBeforeAskingTheCLIToInterrupt reuses
// interrupt_internal_test.go's own live-api-connection fixture so this stays
// end to end through StopChat itself, not a direct call to interruptTurn —
// the ordering under test is StopChat's, and a unit test of interruptTurn
// alone could not see it.
func TestStopChat_RecordsTheStopBeforeAskingTheCLIToInterrupt(t *testing.T) {
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // turn/interrupt
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
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

	require.NoError(t, rs.StopChat(ctx, "chat-1"))

	spy.mu.Lock()
	defer spy.mu.Unlock()
	require.Equal(t, []string{"chat-1"}, spy.recorded,
		"StopChat must record the interruption itself, whether or not the CLI can be asked to cancel in place")
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
