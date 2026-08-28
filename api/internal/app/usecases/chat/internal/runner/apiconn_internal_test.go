package runner

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// noopTurns satisfies the Turns port with the cheapest possible answers —
// nothing here is exercised by this file's tests beyond IngestHook, which
// spyTurns overrides. Mirrors runner_test's own stubTurns, duplicated here
// because that one lives in the external runner_test package.
type noopTurns struct{}

func (noopTurns) IngestHook(context.Context, string, string, string, []byte) error {
	return nil
}

func (noopTurns) ReplayStartupHook(string, inflight.Hook) {}

func (noopTurns) AwaitTurnComplete(context.Context, string) error { return nil }

func (noopTurns) ChatWorking(context.Context, string) (bool, error) { return false, nil }

func (noopTurns) RecordStop(context.Context, string) error { return nil }

func (noopTurns) SetMessageDelta(func(chatID, workspaceID, messageID, text string)) {}

func (noopTurns) MatchTerminalPrompt(
	context.Context, string, string,
) (engineagents.TerminalPrompt, bool) {
	return engineagents.TerminalPrompt{}, false
}

func (noopTurns) MatchTerminalNotice(
	context.Context, string, string,
) (engineagents.TerminalNotice, bool) {
	return engineagents.TerminalNotice{}, false
}

func (noopTurns) OpenWork(context.Context, string) (bool, error) { return false, nil }

func (noopTurns) UnfinishedSince(string) (time.Time, bool) { return time.Time{}, false }

func (noopTurns) AbandonMessage(context.Context, string) (bool, error) { return false, nil }

func (noopTurns) CloseStalledTurn(context.Context, seam.Stall) {}

func TestApiSocketPath_IsShortAndDeterministic(t *testing.T) {
	a := apiSocketPath("runner-1")
	b := apiSocketPath("runner-1")
	c := apiSocketPath("runner-2")

	assert.Equal(t, a, b, "the same runner id must derive the same path")
	assert.NotEqual(t, a, c)
	assert.LessOrEqual(t, len(a), 104, "macOS's sun_path is a hard 104 bytes")
}

func TestWaitForSocket_DetectsTheSocketAppearing(t *testing.T) {
	// A SHORT path, NOT t.TempDir(): that nests under this test's own (long)
	// function name, and macOS's sun_path is a hard 104 bytes — net.Listen fails
	// silently in the background goroutine below if it overflows, and this test
	// then times out waiting for a socket that was never created.
	dir, err := os.MkdirTemp("", "waitforsocket")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "s.sock")

	listening := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		ln, err := net.Listen("unix", sockPath)
		listening <- err
		if err == nil {
			defer ln.Close()
			time.Sleep(200 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.NoError(t, waitForSocket(ctx, sockPath))
	require.NoError(t, <-listening, "the test's own background listener must have bound cleanly")
}

func TestWaitForSocket_RespectsContextCancellation(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, waitForSocket(ctx, filepath.Join(dir, "never.sock")))
}

func TestForkServeProcess_StartsARealProcess(t *testing.T) {
	cmd, err := forkServeProcess([]string{"sleep", "5"})
	require.NoError(t, err)
	require.NotNil(t, cmd.Process)
	defer func() { _ = cmd.Process.Kill() }()

	// The process is genuinely running, not merely constructed.
	assert.NoError(t, cmd.Process.Signal(os.Interrupt))
}

func TestForkServeProcess_EmptyArgvIsAnError(t *testing.T) {
	_, err := forkServeProcess(nil)
	assert.Error(t, err)
}

func TestAPIConnRegistry_DropKillsTheProcessAndClosesTheDriver(t *testing.T) {
	reg := newAPIConnRegistry()
	cmd := exec.Command("sleep", "5")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid

	reg.set("runner-1", &apiconn{serveCmd: cmd})
	reg.drop("runner-1")

	// The process must actually be dead, not merely asked nicely.
	require.Eventually(t, func() bool {
		return cmd.Process.Signal(os.Signal(nil)) != nil || cmd.Wait() != nil
	}, 2*time.Second, 20*time.Millisecond, "process %d should have been killed", pid)

	// A second drop of the same (now-absent) runner must not panic.
	reg.drop("runner-1")
}

func TestAPIConnRegistry_CloseAllKillsEveryLiveProcess(t *testing.T) {
	reg := newAPIConnRegistry()
	cmd1 := exec.Command("sleep", "5")
	require.NoError(t, cmd1.Start())
	cmd2 := exec.Command("sleep", "5")
	require.NoError(t, cmd2.Start())

	reg.set("runner-1", &apiconn{serveCmd: cmd1})
	reg.set("runner-2", &apiconn{serveCmd: cmd2})
	reg.closeAll()

	require.Eventually(t, func() bool {
		return cmd1.Process.Signal(os.Signal(nil)) != nil || cmd1.Wait() != nil
	}, 2*time.Second, 20*time.Millisecond, "process %d should have been killed", cmd1.Process.Pid)
	require.Eventually(t, func() bool {
		return cmd2.Process.Signal(os.Signal(nil)) != nil || cmd2.Wait() != nil
	}, 2*time.Second, 20*time.Millisecond, "process %d should have been killed", cmd2.Process.Pid)

	_, ok := reg.get("runner-1")
	assert.False(t, ok)
	_, ok = reg.get("runner-2")
	assert.False(t, ok)
}

// fakeWSServer starts an in-process WebSocket server on a unix socket, exactly
// mirroring wsrpc/apidriver's own test helpers — used here to build a REAL
// *engineagents.APIConn (via agent.StartAPIConn) without forking a subprocess,
// so pumpAPIConn's own routing logic can be tested in isolation from
// startAPIConn's process-management half.
func fakeWSServer(t *testing.T, afterInit func(*websocket.Conn)) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "apiconn")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	upgrader := websocket.Upgrader{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		_, msg, err := conn.ReadMessage() // initialize
		require.NoError(t, err)
		var req struct{ ID json.RawMessage }
		require.NoError(t, json.Unmarshal(msg, &req))
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]string{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // initialized notification
		afterInit(conn)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(sockPath) })
	return sockPath
}

const apiTransportTestDescriptor = `
id: api-test
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
  permission:
    ask: item/permissions/requestApproval
    timeout_seconds: 270
    map: { tool_name: tool, tool_input: params }
    reply:
      allow: '{"decision":"approved"}'
      deny: '{"decision":"denied","message":{reason_json}}'
  subagent_pre:
    transport: hooks
    in: SubagentStart
    map: { session_id: session_id, subagent_id: agent_id, agent_type: agent_type }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
    handshake: { call: initialize }
`

func apiTransportTestAgent(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "api-test.yaml"), []byte(apiTransportTestDescriptor), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "api-test")
	require.NoError(t, err)
	return a
}

// spyTurns records every IngestHook call, and the delivery id (if any) each one
// carried — the whole surface pumpAPIConn actually exercises.
//
// For an event that carries a delivery id, it ALSO holds a slot on answers —
// mirroring the ordering the real turn package's holdForAnswer guarantees
// (synchronously, before IngestHook returns) so this test's own Resolve call
// cannot race awaitAndReplyOverSocket's Await the way it would if nothing held
// the slot until after pumpAPIConn had already moved on.
type spyTurns struct {
	noopTurns
	answers *answerdesk.Desk

	mu    sync.Mutex
	calls []spyCall
}

type spyCall struct {
	runnerID, provider, canonical, deliveryID string
	raw                                       []byte
}

func (s *spyTurns) IngestHook(
	ctx context.Context, runnerID, provider, canonical string, raw []byte,
) error {
	deliveryID := inflight.DeliveryID(ctx)
	s.mu.Lock()
	s.calls = append(s.calls, spyCall{
		runnerID: runnerID, provider: provider, canonical: canonical,
		deliveryID: deliveryID, raw: raw,
	})
	s.mu.Unlock()
	if deliveryID != "" && s.answers != nil {
		s.answers.Hold(deliveryID, answerdesk.Prompt{
			ChoiceID: "choice-" + deliveryID, ChatID: "chat-1", RunnerID: runnerID,
		})
	}
	return nil
}

// snapshot returns a race-free copy of the calls recorded so far.
func (s *spyTurns) snapshot() []spyCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]spyCall(nil), s.calls...)
}

func TestPumpAPIConn_RoutesOnlyAPITransportEventsAndDropsHooksDeclaredOnes(t *testing.T) {
	turnCompleted := []byte(`{"method":"turn/completed","params":{"threadId":"t1","turn":{"items":[]}}}`)
	// SubagentStart is declared transport: hooks on this descriptor — a driver
	// resolving it must never reach IngestHook, or the hooks wire's own copy
	// would be double-applied.
	subagentStart := []byte(`{"method":"SubagentStart","params":{"session_id":"s1","agent_id":"a1","agent_type":"reviewer"}}`)

	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, turnCompleted))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	// SubagentStart isn't in this descriptor's `in:`/`ask:` table for the API
	// transport at all (it's hooks-only), so dispatch.Resolve never surfaces it —
	// confirming the guard by construction rather than by injecting a frame
	// dispatch would refuse to translate in the first place.
	_ = subagentStart

	spy := &spyTurns{}
	rs := &Runners{turns: spy}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 1 }, 3*time.Second, 10*time.Millisecond)
	calls := spy.snapshot()
	assert.Equal(t, "turn_stop", calls[0].canonical)
	assert.Equal(t, "runner-1", calls[0].runnerID)
	assert.Equal(t, "api-test", calls[0].provider)
	assert.Empty(t, calls[0].deliveryID, "a plain notification carries no delivery id")
}

func TestPumpAPIConn_AskEventCarriesADeliveryIDAndRepliesOverTheSocket(t *testing.T) {
	replySeen := make(chan string, 1)
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		ask, _ := json.Marshal(map[string]any{
			"id": 7, "method": "item/permissions/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var reply struct {
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal(msg, &reply))
		replySeen <- string(reply.Result)
	})

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	answers := answerdesk.New(answerdesk.DefaultRetention, nil)
	spy := &spyTurns{answers: answers}
	rs := &Runners{turns: spy, answers: answers}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 1 }, 3*time.Second, 10*time.Millisecond)
	call := spy.snapshot()[0]
	assert.Equal(t, "permission", call.canonical)
	require.NotEmpty(t, call.deliveryID, "an ask event must carry a delivery id so holdForAnswer can park it")

	// The slot was already held synchronously inside IngestHook (spyTurns), the
	// same ordering the real turn package's holdForAnswer guarantees. Simulate a
	// human deciding it.
	slot, held := answers.ByChoiceID("choice-" + call.deliveryID)
	require.True(t, held)
	answers.Resolve(slot, []byte(`{"decision":"approved"}`))

	select {
	case reply := <-replySeen:
		assert.JSONEq(t, `{"decision":"approved"}`, reply)
	case <-time.After(3 * time.Second):
		t.Fatal("the socket never received a reply")
	}
}

func TestPumpAPIConn_UnansweredAskWritesNoReply(t *testing.T) {
	wroteReply := make(chan struct{}, 1)
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		ask, _ := json.Marshal(map[string]any{
			"id": 9, "method": "item/permissions/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
		_, _, err := conn.ReadMessage()
		if err == nil {
			wroteReply <- struct{}{}
		}
	})

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	spy := &spyTurns{}
	// A retention/wait of practically zero: the relay's declared budget expires
	// almost immediately, so Await returns an empty stdout — "nobody answered in
	// time" — without this test waiting out the real 270s default.
	answers := answerdesk.New(answerdesk.DefaultRetention, nil)
	rs := &Runners{turns: spy, answers: answers}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 1 }, 3*time.Second, 10*time.Millisecond)
	// The slot is already held (spyTurns.IngestHook did it, since spy.answers is
	// unset here it did NOT — hold it now) but never resolved; cancel ctx so
	// awaitAndReplyOverSocket's Await returns promptly via ctx.Done() rather than
	// this test waiting out the real answer-budget timeout.
	answers.Hold(spy.snapshot()[0].deliveryID, answerdesk.Prompt{ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1"})
	cancel()

	select {
	case <-wroteReply:
		t.Fatal("no reply should be written for an ask nobody answered")
	case <-time.After(300 * time.Millisecond):
	}
}
