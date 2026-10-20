package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// stubWorkspaceForInterrupt answers only WorktreeDir — the one call
// interruptTurn makes on its way to resolving the descriptor — and panics on
// anything else via the embedded nil interface, which is the point: a test
// that starts relying on a second method fails loudly instead of silently
// returning zero values.
type stubWorkspaceForInterrupt struct {
	seam.WorkspaceReader
	crowbarHome string
}

func (s stubWorkspaceForInterrupt) WorktreeDir(
	context.Context, string,
) (string, string, string, string, error) {
	return s.crowbarHome, "", "", "", nil
}

// stubAgentsForInterrupt answers only Get, with whatever agent the test built —
// interruptTurn never needs List/RecordInjection/ConsumeInjectedPrefix/ForgetRunner.
type stubAgentsForInterrupt struct {
	engineagents.Agents
	agent engineagents.Agent
}

func (s stubAgentsForInterrupt) Get(
	context.Context, string, string,
) (engineagents.Agent, error) {
	return s.agent, nil
}

func (stubAgentsForInterrupt) ForgetRunner(string) {}

// interruptTestDescriptor mirrors apiTransportTestDescriptor's shape plus an
// interrupt: gesture — a separate constant, not an addition to the shared one,
// so a test wanting NO interrupt declared can still reuse apiTransportTestAgent.
const interruptTestDescriptor = `
id: interrupt-test
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message: "turn.items[type=agentMessage].text"
  interrupt:
    out: turn/interrupt
    send: { threadId: "{session_id}", turnId: "{turn_id}" }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: stdio
    format: ndjson
    serve: [acme, serve]
    handshake: { call: initialize }
`

func interruptTestAgent(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "interrupt-test.yaml"), []byte(interruptTestDescriptor), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "interrupt-test")
	require.NoError(t, err)
	return a
}

func TestInterruptTurn_ReturnsFalse_WhenNoLiveAPIConn(t *testing.T) {
	rs := &Runners{apiConns: newAPIConnRegistry()}
	live := engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"}

	require.False(t, rs.interruptTurn(context.Background(), live))
}

func TestInterruptTurn_ReturnsFalse_WhenDescriptorDeclaresNoInterrupt(t *testing.T) {
	agent := apiTransportTestAgent(t) // the shared fixture: no interrupt: event
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		_, _ = conn.Read() // block; no call should ever arrive
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	rs := &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: agent},
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})
	live := engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "api-test"}

	require.False(t, rs.interruptTurn(ctx, live))
}

// TestInterruptTurn_SendsTheInterruptAndReturnsTrue is StopChat's new path, end
// to end: a live api connection whose descriptor declares interrupt gets the
// wire call, carrying whatever this connection has remembered — the exact
// codex shape (threadId + turnId) confirmed live against codex-cli 0.149.1.
func TestInterruptTurn_SendsTheInterruptAndReturnsTrue(t *testing.T) {
	received := make(chan string, 1)
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		msg, err := conn.Read() // turn/interrupt
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		received <- req.Method + ":" + string(req.Params)
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		require.NoError(t, conn.Write(resp))
		_, _ = conn.Read() // block until the client closes
	})

	agent := interruptTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	rs := &Runners{
		apiConns:   newAPIConnRegistry(),
		ws:         stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:     stubAgentsForInterrupt{agent: agent},
		background: newBackgroundWork(),
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})
	live := engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"}

	require.True(t, rs.interruptTurn(ctx, live))
	// The send runs in the background; closing the connection under it would
	// break the fake server's reply.
	rs.background.wg.Wait()

	select {
	case got := <-received:
		require.Contains(t, got, "turn/interrupt:")
	default:
		t.Fatal("serve never received turn/interrupt")
	}
}

// A provider that never answers the interrupt (a wedged turn) never holds
// Stop: the turn already ended in the ledger, so nothing waits for the reply.
func TestInterruptTurn_NeverWaitsForTheProvidersAnswer(t *testing.T) {
	received := make(chan struct{})
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		_, _ = conn.Read() // the interrupt, never answered
		close(received)
		_, _ = conn.Read()
	})
	agent := interruptTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	rs := &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: agent},
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})
	live := engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test"}

	require.True(t, rs.interruptTurn(ctx, live))
	<-received
}
