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
