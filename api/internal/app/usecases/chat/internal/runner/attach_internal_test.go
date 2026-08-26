package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubRunnerStoreForAttach answers only LiveRunnerForChat — the one call
// SwitchToTerminal/SwitchToNative make on their way to the live runner. The
// embedded nil interface panics on anything else, so a test relying on a
// second method fails loudly instead of silently zero-valuing.
type stubRunnerStoreForAttach struct {
	agentrunner.EventStore
	runner engineagents.Runner
}

func (s stubRunnerStoreForAttach) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	return s.runner, nil
}

// stubTurnsForAttach answers only ChatWorking.
type stubTurnsForAttach struct {
	noopTurns
	working bool
}

func (s stubTurnsForAttach) ChatWorking(context.Context, string) (bool, error) {
	return s.working, nil
}

// stubActivityForAttach answers only LastTurnForSession — the one call
// SwitchToTerminal's new guard makes. The embedded nil interface panics on
// anything else.
type stubActivityForAttach struct {
	agentactivity.EventStore
	found bool
}

func (s stubActivityForAttach) LastTurnForSession(
	context.Context, string, string, string,
) (time.Time, bool, error) {
	return time.Time{}, s.found, nil
}

// fakeTermForAttach is a minimal seam.TerminalCommander: it records every
// CreateCommand call's argv and lets a test trigger onExit synchronously,
// mirroring a real PTY dying on its own.
type fakeTermForAttach struct {
	mu         sync.Mutex
	created    []fakeTermCall
	nextID     int
	onExit     map[string]func()
	terminated []string
}

type fakeTermCall struct {
	workspaceID string
	cwd         string
	argv        []string
	termSessID  string
}

func (f *fakeTermForAttach) CreateCommand(
	_ context.Context, workspaceID, cwd string, argv, _ []string, onExit func(),
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("attach-term-%d", f.nextID)
	f.created = append(f.created, fakeTermCall{workspaceID: workspaceID, cwd: cwd, argv: argv, termSessID: id})
	if f.onExit == nil {
		f.onExit = map[string]func(){}
	}
	f.onExit[id] = onExit
	return id, nil
}

func (f *fakeTermForAttach) TerminateGraceful(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, sessionID)
	return nil
}

func (f *fakeTermForAttach) SessionLive(context.Context, string) bool { return true }

func (f *fakeTermForAttach) lastCall() fakeTermCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.created[len(f.created)-1]
}

func (f *fakeTermForAttach) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

// attachTestDescriptor mirrors interruptTestDescriptor plus a declared
// attach: and a config_injection step, so a test can confirm the attached
// argv carries the SAME hooks wiring a normal hooks-transport spawn gets.
const attachTestDescriptor = `
id: attach-test
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
  prompt:
    fresh:
      - call: thread/start
        send: { cwd: "{cwd}" }
        capture: { session_id: thread.id }
    resume:
      - call: thread/resume
        send: { threadId: "{session_id}", cwd: "{cwd}" }
    action:
      - call: turn/start
        send: { threadId: "{session_id}", text: "{text}" }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize }
config_injection:
  - pass_arg: { arg: "-c", value: 'hooks.Stop=[{hooks=[{type="command",command="{crowbar_hook} hook turn_stop --segment {segid}"}]}]' }
`

func attachTestAgent(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "attach-test.yaml"), []byte(attachTestDescriptor), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "attach-test")
	require.NoError(t, err)
	return a
}

func TestSwitchToTerminal_ReturnsErrNoNativeTerminal_WhenNoLiveAPIConn(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1"}},
	}
	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.ErrorIs(t, err, ErrNoNativeTerminal)
}

// TestSwitchToTerminal_IsIdempotentOnceAlreadyAttached pins a real bug caught
// live: a second SwitchToTerminal call on an already-attached runner has no
// api connection left to check attach against — that must return the SAME
// session, not ErrNoNativeTerminal, since "already switched" is success, not
// the failure that error means everywhere else.
func TestSwitchToTerminal_IsIdempotentOnceAlreadyAttached(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1"}},
	}
	rs.attached.set("runner-1", attachedView{termSessID: "attach-term-1"})

	got, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.Equal(t, "attach-term-1", got)
}

func TestSwitchToTerminal_ReturnsErrTurnInProgress_WhenWorking(t *testing.T) {
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1"}},
		turns:       stubTurnsForAttach{working: true},
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Socket: sockPath, Session: "sess-1", Cwd: "/work"},
	})

	_, err = rs.SwitchToTerminal(context.Background(), "chat-1")
	require.ErrorIs(t, err, ErrTurnInProgress)
	_, stillConnected := rs.apiConns.get("runner-1")
	require.True(t, stillConnected, "a refused switch must not tear anything down")
}

// TestSwitchToTerminal_ForksTheAttachProcessAndDropsTheAPIConnection is the
// happy path: the api connection is torn down, the attach argv (carrying the
// SAME config_injection hooks wiring a normal spawn gets) is forked as a real
// terminal session, and the runner is recorded as attached.
func TestSwitchToTerminal_ForksTheAttachProcessAndDropsTheAPIConnection(t *testing.T) {
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test"},
		},
		turns:    stubTurnsForAttach{working: false},
		activity: stubActivityForAttach{found: true},
		term:     term,
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Socket: sockPath, Session: "sess-1", Cwd: "/work", Segid: "seg-1", CrowbarHook: "/bin/crowbar"},
	})

	termSessID, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.NotEmpty(t, termSessID)

	_, stillConnected := rs.apiConns.get("runner-1")
	require.False(t, stillConnected, "the api connection must be torn down while attached")

	require.Equal(t, 1, term.callCount())
	call := term.lastCall()
	require.Equal(t, "ws-1", call.workspaceID)
	require.Equal(t, "/work", call.cwd)
	require.Equal(t, []string{
		"acme", "resume", "sess-1",
		"-c", `hooks.Stop=[{hooks=[{type="command",command="/bin/crowbar hook turn_stop --segment seg-1"}]}]`,
	}, call.argv, "the attached process must carry the same hooks wiring a normal spawn gets")

	view, ok := rs.attached.get("runner-1")
	require.True(t, ok)
	require.Equal(t, termSessID, view.termSessID)
}

// TestSwitchToTerminal_ReturnsErrNativeViewNotYetAvailable_WhenSessionNeverCompletedATurn
// pins the fix for a bug that reached a real user: switching to Terminal before
// the first exchange completes forks a codex resume that dies within
// milliseconds — codex writes no rollout for a thread until a turn completes —
// and nothing caught that, so the chat's terminal silently fell back to the
// disconnected companion PTY every api-transport spawn still forks. A user who
// typed into THAT created a completely independent codex session that got
// silently promoted into its own new chat the moment it announced itself
// (MoveToNew, move.go). Confirmed live end to end.
func TestSwitchToTerminal_ReturnsErrNativeViewNotYetAvailable_WhenSessionNeverCompletedATurn(t *testing.T) {
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test"},
		},
		turns:    stubTurnsForAttach{working: false},
		activity: stubActivityForAttach{found: false},
		term:     term,
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Socket: sockPath, Session: "sess-1", Cwd: "/work", Segid: "seg-1", CrowbarHook: "/bin/crowbar"},
	})

	_, err = rs.SwitchToTerminal(context.Background(), "chat-1")
	require.ErrorIs(t, err, ErrNativeViewNotYetAvailable)
	_, stillConnected := rs.apiConns.get("runner-1")
	require.True(t, stillConnected, "a refused switch must not tear anything down")
	require.Equal(t, 0, term.callCount(), "nothing must be forked when the refusal fires first")
}

func TestSwitchToNative_IsANoop_WhenNothingAttached(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{runner: engineagents.Runner{ID: "runner-1"}},
	}
	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))
}

// TestSwitchToNative_TerminatesAndReestablishes proves the reversible half:
// SwitchToNative kills the attached PTY, clears the attached record, and
// ATTEMPTS to re-establish the api connection via applyAPITransport — the
// exact same call the real spawn path makes, so it forks a real subprocess
// and cannot be driven by this file's in-process fake server (unlike a
// hand-built *engineagents.APIConn, which the OTHER tests in this package use
// specifically to avoid that). Its own doc comment already treats a failed
// reconnect as "leave the chat dormant, never fail the caller" — degrading
// exactly like a fresh spawn's applyAPITransport would — so this test asserts
// the two things that don't require a real subprocess: the PTY is actually
// terminated, and SwitchToNative itself never errors even when the reconnect
// it attempts cannot succeed in this environment.
func TestSwitchToNative_TerminatesAndReestablishes(t *testing.T) {
	agent := attachTestAgent(t)
	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test"},
		},
		turns: stubTurnsForAttach{working: false},
		term:  term,
	}
	// Registered directly, as if SwitchToTerminal had already run and torn the
	// api connection down — SwitchToNative's own job is reversing that.
	rs.attached.set("runner-1", attachedView{
		termSessID: "attach-term-1",
		agent:      agent,
		tctx:       engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work"},
	})

	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))

	require.Contains(t, term.terminated, "attach-term-1")
	_, stillAttached := rs.attached.get("runner-1")
	require.False(t, stillAttached, "the attached record must clear regardless of whether reconnecting succeeds")
}
