package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// noopTurns is the Turns port doing nothing; spyTurns overrides its Hook.
type noopTurns struct{}

func (noopTurns) Hook(context.Context, string, string, string, []byte) (string, error) {
	return "", nil
}

func (noopTurns) ForgetChat(string) {}

func (noopTurns) RecordStop(context.Context, string) error { return nil }

func (noopTurns) RecordChatSwitch(context.Context, string, string, string) error { return nil }

func (noopTurns) SetFeed(seam.ChatFeed) {}

func (noopTurns) MatchTerminalPrompt(context.Context, string, string) (engineagents.TerminalPrompt, bool) {
	return engineagents.TerminalPrompt{}, false
}

func (noopTurns) MatchTerminalNotice(context.Context, string, string) (engineagents.TerminalNotice, bool) {
	return engineagents.TerminalNotice{}, false
}

func (noopTurns) AbandonMessageForRunner(context.Context, string, engineagents.Runner) (bool, error) {
	return false, nil
}

func (noopTurns) FailTurnOnNotice(context.Context, engineagents.Runner, engineagents.TerminalNotice) {
}

func TestAPIConnRegistry_DropKillsTheProcessAndClosesTheDriver(t *testing.T) {
	reg := newAPIConnRegistry()
	cmd := exec.Command("sleep", "5")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid

	serve := reapServe(cmd)
	reg.set("runner-1", &apiconn{serve: serve})
	reg.drop("runner-1")

	// Gone by the time drop returns: codex refuses a replacement resuming the
	// thread while the old process still holds its writer lock.
	select {
	case <-serve.exited:
	default:
		t.Fatal("drop returned before the killed serve process was reaped")
	}

	// The process must actually be dead, not merely asked nicely.
	require.Eventually(t, func() bool {
		return cmd.Process.Signal(os.Signal(nil)) != nil || cmd.Wait() != nil
	}, 2*time.Second, 20*time.Millisecond, "process %d should have been killed", pid)

	// A second drop of the same (now-absent) runner must not panic.
	reg.drop("runner-1")
}

// A daemon shutting down kills its own serve processes; that is not the
// runner exiting. Its row stays live, so the next boot records the real
// reason (daemon_restart) — an exit reconciled here raced the store's close.
func TestAPIConnRegistry_CloseAllIsNotARunnerExit(t *testing.T) {
	reg := newAPIConnRegistry()
	cmd := exec.CommandContext(t.Context(), "sleep", "5")
	require.NoError(t, cmd.Start())
	serve := reapServe(cmd)
	reg.set("runner-1", &apiconn{serve: serve})
	exited := make(chan struct{}, 1)
	require.True(t, reg.watchExit("runner-1", func(string) { exited <- struct{}{} }))

	reg.closeAll()

	<-serve.exited
	select {
	case <-exited:
		t.Fatal("the daemon's own shutdown was reconciled as the runner exiting")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAPIConnRegistry_CloseAllKillsEveryLiveProcess(t *testing.T) {
	reg := newAPIConnRegistry()
	cmd1 := exec.Command("sleep", "5")
	require.NoError(t, cmd1.Start())
	cmd2 := exec.Command("sleep", "5")
	require.NoError(t, cmd2.Start())

	reg.set("runner-1", &apiconn{serve: reapServe(cmd1)})
	reg.set("runner-2", &apiconn{serve: reapServe(cmd2)})
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

// helperServeGate names a FIFO the helper serve opens before it reads a word:
// the open proves serve is alive, and the test's close lets it answer.
const helperServeGate = "CROWBAR_TEST_SERVE_GATE"

func awaitServeGate(t *testing.T) {
	t.Helper()
	gate := os.Getenv(helperServeGate)
	if gate == "" {
		return
	}
	f, err := os.Open(gate)
	require.NoError(t, err)
	_, err = f.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, f.Close())
}

// Serve is ready when it answers the handshake: one that is alive but silent
// has given its runner neither a connection nor a PTY.
func TestStartAPIConn_ServeIsReadyOnlyOnceItAnswers(t *testing.T) {
	h := helperServeSpawn(t, "answer", false)
	gate := filepath.Join(t.TempDir(), "gate")
	require.NoError(t, syscall.Mkfifo(gate, 0o600))
	t.Setenv(helperServeGate, gate)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	spawned := make(chan error, 1)
	go func() {
		_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test", spawnInput{runnerID: "runner-1"}, false)
		spawned <- err
	}()
	opened := make(chan *os.File, 1)
	go func() {
		f, err := os.OpenFile(gate, os.O_WRONLY, 0) // returns once serve opens its end
		if assert.NoError(t, err) {
			opened <- f
		}
	}()
	select {
	case release := <-opened:
		assert.False(t, h.rs.HasLiveAPIConnection("runner-1"), "a silent serve was taken as ready")
		assert.Zero(t, h.term.callCount(), "a silent serve was given up for a PTY")
		require.NoError(t, release.Close())
	case err := <-spawned:
		t.Fatalf("the spawn went on before its serve answered: %v", err)
	case <-ctx.Done():
		t.Fatal("serve never started")
	}

	require.NoError(t, <-spawned)
	assert.True(t, h.rs.HasLiveAPIConnection("runner-1"))
	assert.Empty(t, h.term.created, "the answered connection is the runner")
}

// A serve that ends without answering was never ready: its end is seen at
// once, and the spawn goes on over its PTY.
func TestStartAPIConn_AServeThatEndsUnansweredIsNotReady(t *testing.T) {
	h := helperServeSpawn(t, "exit", false)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test", spawnInput{runnerID: "runner-1"}, false)

	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "serve's end went unseen until the spawn's bound")
	assert.False(t, h.rs.HasLiveAPIConnection("runner-1"))
	assert.Len(t, h.term.created, 1, "the spawn went on over its PTY")
}

// fakeServe is serve on the far end of a stdio pipe, answering the handshake
// as a real one does: a REAL *engineagents.APIConn (via agent.StartAPIConn)
// without forking a process, so pumpAPIConn's own routing is tested apart from
// startAPIConn's process half. afterInit then holds the child's end.
func fakeServe(t *testing.T, afterInit func(*testutil.LinePeer)) engineagents.APIStdio {
	t.Helper()
	in, out, peer := testutil.StdioPipe(t)
	go func() {
		var req struct{ ID json.RawMessage }
		if peer.ReadJSON(&req) != nil || // initialize; a failed read is the client hanging up
			!assert.NoError(t, peer.WriteJSON(map[string]any{"id": req.ID, "result": map[string]string{}})) {
			return
		}
		if _, err := peer.Read(); err == nil && afterInit != nil { // initialized notification
			afterInit(peer)
		}
	}()
	return engineagents.APIStdio{In: in, Out: out}
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
    ask: acme/tool/requestApproval
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
    delivery: stdio
    format: ndjson
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

// spyTurns records every event the pump places. Like the real ingress, it
// holds a permission's ask before Hook returns, so the test's Resolve cannot
// race awaitAndReply's Await; failFirst refuses the first event placed.
type spyTurns struct {
	noopTurns
	answers   *answerdesk.Desk
	failFirst bool

	mu    sync.Mutex
	calls []spyCall
}

type spyCall struct {
	runnerID, provider, canonical, ask string
	raw                                []byte
}

func (s *spyTurns) Hook(
	_ context.Context, runnerID, provider, canonical string, raw []byte,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	call := spyCall{runnerID: runnerID, provider: provider, canonical: canonical, raw: raw}
	if canonical == engineagents.HookPermission && s.answers != nil {
		call.ask = fmt.Sprintf("ask-%d", len(s.calls))
		s.answers.Hold(answerdesk.Prompt{ChoiceID: call.ask, ChatID: "chat-1", RunnerID: runnerID})
	}
	s.calls = append(s.calls, call)
	if s.failFirst && len(s.calls) == 1 {
		return "", errors.New("unrecorded")
	}
	return call.ask, nil
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

	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		require.NoError(t, conn.Write(turnCompleted))
		_, _ = conn.Read() // block until the client closes
	})

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	// SubagentStart isn't in this descriptor's `in:`/`ask:` table for the API
	// transport at all (it's hooks-only), so dispatch.Resolve never surfaces it —
	// confirming the guard by construction rather than by injecting a frame
	// dispatch would refuse to translate in the first place.
	_ = subagentStart

	spy := &spyTurns{}
	rs := &Runners{turns: spy, live: newLiveTable()}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 1 }, 3*time.Second, 10*time.Millisecond)
	calls := spy.snapshot()
	assert.Equal(t, "turn_stop", calls[0].canonical)
	assert.Equal(t, "runner-1", calls[0].runnerID)
	assert.Equal(t, "api-test", calls[0].provider)
	assert.Empty(t, calls[0].ask, "a plain notification holds no ask")
}

func TestPumpAPIConn_AnAskIsAnsweredOverTheConnection(t *testing.T) {
	replySeen := make(chan string, 1)
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		ask, _ := json.Marshal(map[string]any{
			"id": 7, "method": "acme/tool/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.Write(ask))
		msg, err := conn.Read()
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
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	answers := answerdesk.New(answerdesk.DefaultRetention, nil)
	spy := &spyTurns{answers: answers}
	rs := &Runners{turns: spy, answers: answers, live: newLiveTable()}
	conn := &apiconn{driver: apiConn, ctx: ctx}
	rs.pumpAPIConn("runner-1", "api-test", agent, conn)

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 1 }, 3*time.Second, 10*time.Millisecond)
	call := spy.snapshot()[0]
	assert.Equal(t, "permission", call.canonical)
	require.NotEmpty(t, call.ask, "an ask event is held on its ask so the pump can await it")

	// Held inside Hook (spyTurns), as the real ingress does: a human decides it.
	slot, held := answers.ByChoiceID(call.ask)
	require.True(t, held)
	answers.Resolve(slot, []byte(`{"decision":"approved"}`))

	select {
	case reply := <-replySeen:
		assert.JSONEq(t, `{"decision":"approved"}`, reply)
	case <-time.After(3 * time.Second):
		t.Fatal("serve never received a reply")
	}
}

// An api provider has no TUI to fall back to: an ask nobody answers from
// Crowbar, or one the ingress refused, gets the descriptor's refusal, never
// silence that holds its turn.
func TestPumpAPIConn_AnAskNobodyAnswersIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		turns  *spyTurns
		reason string
	}{
		"held by nobody":         {turns: &spyTurns{}, reason: "No answer from Crowbar in time."},
		"refused by the ingress": {turns: &spyTurns{failFirst: true}, reason: "Crowbar could not record this request."},
	} {
		t.Run(name, func(t *testing.T) {
			replySeen := make(chan string, 1)
			stdio := fakeServe(t, func(conn *testutil.LinePeer) {
				ask, _ := json.Marshal(map[string]any{
					"id": 9, "method": "acme/tool/requestApproval", "params": map[string]string{"tool": "shell"},
				})
				require.NoError(t, conn.Write(ask))
				raw, err := conn.Read()
				if err != nil {
					return
				}
				var reply struct {
					Result json.RawMessage `json:"result"`
				}
				require.NoError(t, json.Unmarshal(raw, &reply))
				replySeen <- string(reply.Result)
				_, _ = conn.Read()
			})
			agent := apiTransportTestAgent(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
			require.NoError(t, err)
			defer func() { _ = apiConn.Close() }()

			rs := &Runners{turns: tc.turns, answers: answerdesk.New(answerdesk.DefaultRetention, nil), live: newLiveTable()}
			rs.pumpAPIConn("runner-1", "api-test", agent, &apiconn{driver: apiConn, ctx: ctx})

			select {
			case reply := <-replySeen:
				assert.JSONEq(t, fmt.Sprintf(`{"decision":"denied","message":%q}`, tc.reason), reply)
			case <-ctx.Done():
				t.Fatal("an unanswered ask must be refused, not left open")
			}
		})
	}
}

// An ask is awaited off the pump: every event behind it (deltas, turn_stop, the
// next ask) keeps flowing while a human decides, and an interrupt never hangs
// behind the driver's full buffers.
func TestPumpAPIConn_AnUnansweredAskDoesNotBlockLaterEvents(t *testing.T) {
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		ask, _ := json.Marshal(map[string]any{
			"id": 11, "method": "acme/tool/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.Write(ask))
		require.NoError(t, conn.Write(
			[]byte(`{"method":"turn/completed","params":{"threadId":"t1","turn":{"items":[]}}}`)))
		_, _ = conn.Read()
	})

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	answers := answerdesk.New(answerdesk.DefaultRetention, nil)
	spy := &spyTurns{answers: answers}
	rs := &Runners{turns: spy, answers: answers, live: newLiveTable()}
	rs.pumpAPIConn("runner-1", "api-test", agent, &apiconn{driver: apiConn, ctx: ctx})

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 2 }, 3*time.Second, 10*time.Millisecond,
		"the turn_stop behind an unanswered permission ask must still be ingested")
	assert.Equal(t, "turn_stop", spy.snapshot()[1].canonical)
}

// An event the ingress could not place is already dead-lettered: the pump goes
// on placing the ones behind it.
func TestPumpAPIConn_KeepsPlacingAfterAnEventIsRefused(t *testing.T) {
	turnCompleted := []byte(`{"method":"turn/completed","params":{"threadId":"t1","turn":{"items":[]}}}`)
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		require.NoError(t, conn.Write(turnCompleted))
		require.NoError(t, conn.Write(turnCompleted))
		_, _ = conn.Read()
	})

	agent := apiTransportTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	spy := &spyTurns{failFirst: true}
	rs := &Runners{turns: spy, live: newLiveTable()}
	rs.pumpAPIConn("runner-1", "api-test", agent, &apiconn{driver: apiConn, ctx: ctx})

	require.Eventually(t, func() bool { return len(spy.snapshot()) == 2 }, 3*time.Second, 10*time.Millisecond,
		"the event behind a refused one must still be placed")
}
