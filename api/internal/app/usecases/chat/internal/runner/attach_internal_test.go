package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type fakeTermForAttach struct {
	mu         sync.Mutex
	created    []fakeTermCall
	nextID     int
	onExit     map[string]func(engineterminal.Exit)
	terminated []string
}

type fakeTermCall struct {
	chatID     string
	cwd        string
	argv       []string
	env        []string
	termSessID string
}

func (f *fakeTermForAttach) CreateCommand(
	_ context.Context, chatID, cwd string, argv, env []string, onExit func(engineterminal.Exit),
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("attach-term-%d", f.nextID)
	f.created = append(f.created,
		fakeTermCall{chatID: chatID, cwd: cwd, argv: argv, env: env, termSessID: id})
	if f.onExit == nil {
		f.onExit = map[string]func(engineterminal.Exit){}
	}
	f.onExit[id] = onExit
	return id, nil
}

func (f *fakeTermForAttach) Write(context.Context, string, []byte) error { return nil }

func (f *fakeTermForAttach) TerminateGraceful(_ context.Context, sessionID string) error {
	f.mu.Lock()
	f.terminated = append(f.terminated, sessionID)
	onExit, ok := f.onExit[sessionID]
	delete(f.onExit, sessionID)
	f.mu.Unlock()
	if !ok {
		return engineterminal.ErrSessionNotFound
	}
	onExit(engineterminal.Exit{Code: 143})
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
        send: { cwd: "{cwd}", developerInstructions: "{context}" }
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
    delivery: stdio
    format: ndjson
    serve:  [acme, app-server]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize, notify: initialized }
config_injection:
  - pass_arg: { arg: "-c", value: 'hooks.Stop=[{hooks=[{type="command",command="{hook_url}/turn_stop"}]}]' }
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
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live: liveWith(t, engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"}),
	}
	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.ErrorIs(t, err, ErrNoNativeTerminal)
}

func TestSwitchToTerminal_IsIdempotentOnceAlreadyAttached(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live: liveWith(t, engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"}),
	}
	rs.attached.set("runner-1", attachedView{termSessID: "attach-term-1"})

	got, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.Equal(t, "attach-term-1", got)
}

func TestSwitchToTerminal_ReturnsErrTurnInProgress_WhenWorking(t *testing.T) {
	stdio := fakeServe(t, nil)
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live:   liveWith(t, engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"}),
		turns:  noopTurns{},
		ledger: runningTestLedger(t),
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work"},
	})

	_, err = rs.SwitchToTerminal(context.Background(), "chat-1")
	require.ErrorIs(t, err, ErrTurnInProgress)
	_, stillConnected := rs.apiConns.get("runner-1")
	require.True(t, stillConnected, "a refused switch must not tear anything down")
}

func TestSwitchToTerminal_ForksTheAttachProcessAndDropsTheAPIConnection(t *testing.T) {
	stdio := fakeServe(t, nil)
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live:   liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentChatID: "chat-1"}),
		turns:  noopTurns{},
		term:   term,
		ledger: turnedTestLedger(t, "attach-test", "sess-1"),
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work", Segid: "seg-1", HookBase: "http://h", ProjectID: "p", Provider: "acme"},
	})

	termSessID, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.NotEmpty(t, termSessID)

	_, stillConnected := rs.apiConns.get("runner-1")
	require.False(t, stillConnected, "the api connection must be torn down while attached")

	require.Equal(t, 1, term.callCount())
	call := term.lastCall()
	require.Equal(t, "chat-1", call.chatID,
		"the forked attach session is owned by the CHAT it was switched from")
	require.Equal(t, "/work", call.cwd)
	require.Equal(t, []string{
		"acme", "resume", "sess-1",
		"-c", `hooks.Stop=[{hooks=[{type="command",command="http://h/v0/projects/p/home/chats/runners/seg-1/hooks/acme/turn_stop"}]}]`,
	}, call.argv, "the attached process must carry the same hooks wiring a normal spawn gets")

	view, ok := rs.attached.get("runner-1")
	require.True(t, ok)
	require.Equal(t, termSessID, view.termSessID)

	require.Contains(t, call.env, "PATH="+os.Getenv("PATH"),
		"the native view must inherit the daemon's environment, or its hooks cannot run")
}

func TestRegression_SwitchToTerminalForksWithTheProcessEnvironment(t *testing.T) {
	stdio := fakeServe(t, nil)
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live:   liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentChatID: "chat-1"}),
		turns:  noopTurns{},
		term:   term,
		ledger: turnedTestLedger(t, "attach-test", "sess-1"),
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work", Segid: "seg-1", CrowbarBin: "/bin/crowbar"},
	})

	_, err = rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)

	require.NotEmpty(t, term.lastCall().env, "a nil env is what left the native view without a PATH")
	require.Subset(t, term.lastCall().env, os.Environ())
}

func TestRegression_SwitchToTerminal_ChecksTheBoundSession_NotStaleAPIConnSession(t *testing.T) {
	for name, turned := range map[string]bool{"turned": true, "never turned": false} {
		t.Run(name, func(t *testing.T) {
			l := boundTestLedger(t, "attach-test", "durable-sess")
			if turned {
				l = turnedTestLedger(t, "attach-test", "durable-sess")
			}
			switchToTerminalOnTheBoundSession(t, l, turned)
		})
	}
}

func switchToTerminalOnTheBoundSession(t *testing.T, l *ledgerstore.Store, attached bool) {
	t.Helper()
	stdio := fakeServe(t, nil)
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live:   liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentChatID: "chat-1"}),
		turns:  noopTurns{},
		term:   term,
		ledger: l,
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Session: "stale-conn-sess", Cwd: "/work", Segid: "seg-1", CrowbarBin: "/bin/crowbar"},
	})

	termSessID, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	if !attached {
		require.ErrorIs(t, err, ErrNoNativeTerminal, "a session that never turned cannot be loaded by the TUI")
		require.Zero(t, term.callCount())
		return
	}
	require.NoError(t, err)
	require.NotEmpty(t, termSessID)
	require.Equal(t, 1, term.callCount(), "the guard's stale-session check must not block a session that has turned")
	require.Contains(t, term.created[0].argv, "durable-sess")
}

func TestSwitchToNative_IsANoop_WhenNothingAttached(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live: liveWith(t, engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"}),
	}
	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))
}

func TestSwitchToNative_TerminatesAndReestablishes(t *testing.T) {
	agent := attachTestAgent(t)
	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live:    liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentChatID: "chat-1"}),
		turns:   noopTurns{},
		term:    term,
		agents:  engineagents.New(),
		answers: answerdesk.New(answerdesk.DefaultRetention, noLedger{}),
		ledger:  newTestLedger(t),
	}
	rs.attached.set("runner-1", attachedView{
		termSessID: "attach-term-1",
		agent:      agent,
		tctx:       engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work"},
	})
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))

	require.Contains(t, term.terminated, "attach-term-1")
	_, stillAttached := rs.attached.get("runner-1")
	require.False(t, stillAttached, "the attached record must clear regardless of whether reconnecting succeeds")
	_, live := rs.RunnerByID("runner-1")
	require.False(t, live, "the view is dead and no connection replaced it; the runner has no process left to be")
	exit, ok := chatState(t, rs.ledger).LastExit()
	require.True(t, ok)
	require.Equal(t, "runner-1", exit.RunnerID)
}

type phaseTouches struct {
	rs   *Runners
	seen []string
}

func (p *phaseTouches) Touch(_ context.Context, chatID string) {
	p.seen = append(p.seen, p.rs.Phase(chatID))
}

func TestRegression_SwitchToNativeReadsAsSwitchingUntilItLands(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(), phases: newPhases(),
		live:    liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentChatID: "chat-1"}),
		turns:   noopTurns{},
		term:    &fakeTermForAttach{},
		agents:  engineagents.New(),
		answers: answerdesk.New(answerdesk.DefaultRetention, noLedger{}),
		ledger:  newTestLedger(t),
	}
	touches := &phaseTouches{rs: rs}
	rs.snapshots = touches
	rs.attached.set("runner-1", attachedView{
		termSessID: "attach-term-1", agent: attachTestAgent(t),
		tctx: engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work"},
	})
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))

	require.NotEmpty(t, touches.seen)
	assert.Equal(t, snapshot.PhaseSwitching, touches.seen[0], "the switch was not announced before it waited")
	assert.Empty(t, rs.Phase("chat-1"), "the switch never ended")
}

func TestRegression_SwitchToNativeRefusesWhileTheAttachedViewIsWorking(t *testing.T) {
	term := &fakeTermForAttach{}
	rs := &Runners{
		attached: newAttachRegistry(), chatLock: newTestLock(), parks: newParks(),
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		live:   liveWith(t, engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"}),
		turns:  noopTurns{},
		ledger: runningTestLedger(t),
		term:   term,
	}
	rs.attached.set("runner-1", attachedView{termSessID: "attach-term-1"})

	err := rs.SwitchToNative(context.Background(), "chat-1")

	require.ErrorIs(t, err, ErrTurnInProgress)
	require.Empty(t, term.terminated, "a refused switch must not kill the view")
	_, stillAttached := rs.attached.get("runner-1")
	require.True(t, stillAttached)
}
