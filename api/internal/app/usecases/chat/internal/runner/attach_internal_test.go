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

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
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
	// exited records every Exit. A runner whose process set is empty is now
	// reconciled from the teardown paths themselves (exitProcesslessRunner),
	// so these tests need to be able to see that happen.
	exited chan string
}

func (s stubRunnerStoreForAttach) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	return s.runner, nil
}

func (s stubRunnerStoreForAttach) Get(
	_ context.Context, runnerID string,
) (engineagents.Runner, error) {
	if s.runner.ID != runnerID {
		return engineagents.Runner{}, agentrunner.ErrNotFound
	}
	return s.runner, nil
}

func (s stubRunnerStoreForAttach) Exit(
	_ context.Context, runnerID string, _ time.Time,
) (engineagents.Runner, error) {
	if s.exited != nil {
		s.exited <- runnerID
	}
	return s.runner, nil
}

// stubTurnsForAttach answers only ChatWorking and TurnOpen.
type stubTurnsForAttach struct {
	noopTurns
	working bool
}

func (s stubTurnsForAttach) ChatWorking(context.Context, string) (bool, error) {
	return s.working, nil
}

func (s stubTurnsForAttach) TurnOpen(context.Context, string, string) (bool, error) {
	return s.working, nil
}

// stubActivityForAttach answers LastTurnForSession and CountTurns — the calls
// SwitchToTerminal's guard and resumableConversation's legacy-vs-crash check
// make. The embedded nil interface panics on anything else. turnCount
// defaults to zero, which is what every pre-existing test wants (a chat with
// no other recorded activity), so callers that don't care about it need no
// changes.
type stubActivityForAttach struct {
	agentactivity.EventStore
	found     bool
	turnCount int64
}

func (s stubActivityForAttach) LastTurnForSession(
	context.Context, string, string, string,
) (time.Time, bool, error) {
	return time.Time{}, s.found, nil
}

func (s stubActivityForAttach) CountTurns(context.Context, string) (int64, error) {
	return s.turnCount, nil
}

// stubActivityBySession answers LastTurnForSession by exact sessionID match
// against wantSession — unlike stubActivityForAttach's session-agnostic
// canned bool, this lets a test tell apart a call made with the runner row's
// durable CurrentSession from one made with an apiconn's own tctx.Session.
type stubActivityBySession struct {
	agentactivity.EventStore
	wantSession string
}

func (s stubActivityBySession) LastTurnForSession(
	_ context.Context, _, _, sessionID string,
) (time.Time, bool, error) {
	return time.Time{}, sessionID == s.wantSession, nil
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
	chatID     string
	cwd        string
	argv       []string
	env        []string
	termSessID string
}

func (f *fakeTermForAttach) CreateCommand(
	_ context.Context, chatID, cwd string, argv, env []string, onExit func(),
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("attach-term-%d", f.nextID)
	f.created = append(f.created,
		fakeTermCall{chatID: chatID, cwd: cwd, argv: argv, env: env, termSessID: id})
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
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
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
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
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
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer apiConn.Close()

	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
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
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer apiConn.Close()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{
				ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentSession: "sess-1",
			},
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
	require.Equal(t, "chat-1", call.chatID,
		"the forked attach session is owned by the CHAT it was switched from")
	require.Equal(t, "/work", call.cwd)
	require.Equal(t, []string{
		"acme", "resume", "sess-1",
		"-c", `hooks.Stop=[{hooks=[{type="command",command="/bin/crowbar hook turn_stop --segment seg-1"}]}]`,
	}, call.argv, "the attached process must carry the same hooks wiring a normal spawn gets")

	view, ok := rs.attached.get("runner-1")
	require.True(t, ok)
	require.Equal(t, termSessID, view.termSessID)

	// TestRegression_SwitchToTerminalForksWithTheProcessEnvironment's fact, asserted
	// on the happy path it belongs to: see that test's own comment.
	require.Contains(t, call.env, "PATH="+os.Getenv("PATH"),
		"the native view must inherit the daemon's environment, or its hooks cannot run")
}

// TestRegression_SwitchToTerminalForksWithTheProcessEnvironment pins a bug
// measured live: CreateCommand takes its env VERBATIM, and this path passed
// nil, so the native view ran with only the terminal defaults — no PATH, no
// HOME. Every hook APIAttachArgv wires then died with exit 127 and `crowbar
// mcp` never started, so a chat handed to its provider's own view recorded
// nothing at all: the exact failure APIAttachArgv's doc says it exists to
// prevent.
func TestRegression_SwitchToTerminalForksWithTheProcessEnvironment(t *testing.T) {
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer apiConn.Close()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{
				ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test", CurrentSession: "sess-1",
			},
		},
		turns:    stubTurnsForAttach{working: false},
		activity: stubActivityForAttach{found: true},
		term:     term,
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Socket: sockPath, Session: "sess-1", Cwd: "/work", Segid: "seg-1", CrowbarHook: "/bin/crowbar"},
	})

	_, err = rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)

	require.NotEmpty(t, term.lastCall().env, "a nil env is what left the native view without a PATH")
	require.Subset(t, term.lastCall().env, os.Environ())
}

// The attach decision reads the runner row's CurrentSession, under which
// turns are recorded, not the connection's establish-time copy: a session
// that completed a turn attaches even when the two differ.
func TestRegression_SwitchToTerminal_ChecksRunnerCurrentSession_NotStaleAPIConnSession(t *testing.T) {
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	defer apiConn.Close()

	term := &fakeTermForAttach{}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{
				ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test",
				CurrentSession: "durable-sess", // the id turns are actually recorded under
			},
		},
		turns:    stubTurnsForAttach{working: false},
		activity: stubActivityBySession{wantSession: "durable-sess"},
		term:     term,
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		// tctx.Session deliberately differs from CurrentSession — the stale,
		// establish-time copy the pre-fix guard checked instead.
		tctx: engineagents.TemplateCtx{Socket: sockPath, Session: "stale-conn-sess", Cwd: "/work", Segid: "seg-1", CrowbarHook: "/bin/crowbar"},
	})

	termSessID, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.NotEmpty(t, termSessID)
	require.Equal(t, 1, term.callCount(), "the guard's stale-session check must not block a session that has turned")
}

func TestSwitchToNative_IsANoop_WhenNothingAttached(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
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
// the three things that don't require a real subprocess: the PTY is actually
// terminated, SwitchToNative itself never errors even when the reconnect it
// attempts cannot succeed in this environment, and the runner — which has no
// PTY of its own and now no connection either — is reconciled as gone rather
// than left advertising a live agent with no process behind it.
func TestSwitchToNative_TerminatesAndReestablishes(t *testing.T) {
	agent := attachTestAgent(t)
	term := &fakeTermForAttach{}
	exited := make(chan string, 1)
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(), spawns: inflight.NewGate(),
		// surfaces/chats: the two switch calls now MOVE the chat's current
		// surface (domain.Chat.Surface), in memory and durably.
		surfaces: newSurfaceRegistry(), chats: newSpySurfaceChats(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "attach-test"},
			exited: exited,
		},
		turns:         stubTurnsForAttach{working: false},
		term:          term,
		agents:        engineagents.New(),
		answers:       answerdesk.New(answerdesk.DefaultRetention, nil),
		inflightTurns: inflight.NewTurns(),
		work:          inflight.NewWork(),
		activity:      stubActivityForSpawn{},
		prompts:       agentjournal.NewPromptRequests(),
		home:          func() (string, error) { return t.TempDir(), nil },
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
	select {
	case got := <-exited:
		require.Equal(t, "runner-1", got)
	default:
		t.Fatal("the view is dead and no connection replaced it; the runner has no process left to be")
	}
}

// spySurfaceChats records every SetSurface write, so a test can assert on the
// DURABLE surface rather than only on the in-process mirror of it.
type spySurfaceChats struct {
	stubChatsForSpawn
	written chan string
}

func newSpySurfaceChats() *spySurfaceChats {
	return &spySurfaceChats{
		stubChatsForSpawn: stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		written:           make(chan string, 4),
	}
}

// lastWritten is a NON-BLOCKING read: the write happens inside the call under
// test and has already returned by the time a test asks, so an empty channel
// means the write never happened — never that it has not happened yet.
func (s *spySurfaceChats) lastWritten(t *testing.T) string {
	t.Helper()
	select {
	case surface := <-s.written:
		return surface
	default:
		t.Fatal("nothing wrote the chat's current surface")
		return ""
	}
}

func (s *spySurfaceChats) SetSurface(
	_ context.Context, _, surface string,
) (domain.Chat, error) {
	s.written <- surface
	return s.chat, nil
}

// TestRegression_SwitchToTerminalWritesTheChatsCurrentSurface: the switch
// endpoints are the two moments Crowbar is actually TOLD the user moved, so
// they are what makes domain.Chat.Surface a current fact instead of a birth
// record. Without this the chat came back from a daemon restart (or any
// respawn) believing it was still on the surface it was created on, and
// rebuilt the wrong transport underneath the view the user was looking at.
func TestRegression_SwitchToTerminalWritesTheChatsCurrentSurface(t *testing.T) {
	rs, chats := switchSurfaceFixture(t)

	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)

	require.Equal(t, domain.SurfaceTerminal, chats.lastWritten(t))
	require.True(t, rs.ShowingNativeView("runner-1"),
		"the in-process mirror moves with the durable field, never independently of it")
}

// TestRegression_SwitchToNativeWritesTheChatsCurrentSurface is the reverse:
// a chat handed back to Crowbar's own chat must stop reporting a native view.
func TestRegression_SwitchToNativeWritesTheChatsCurrentSurface(t *testing.T) {
	rs, chats := switchSurfaceFixture(t)

	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.Equal(t, domain.SurfaceTerminal, chats.lastWritten(t))

	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))

	require.Equal(t, domain.SurfaceChat, chats.lastWritten(t))
	require.False(t, rs.ShowingNativeView("runner-1"))
}

// switchSurfaceFixture stands up the smallest Runners that can complete a
// real SwitchToTerminal: a live api connection over this file's in-process
// server, a completed turn on the runner's own session, and an idle chat.
func switchSurfaceFixture(t *testing.T) (*Runners, *spySurfaceChats) {
	t.Helper()
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) { _, _, _ = conn.ReadMessage() })
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	apiConn, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = apiConn.Close() })

	chats := newSpySurfaceChats()
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(),
		surfaces: newSurfaceRegistry(), spawns: inflight.NewGate(),
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{
				ID: "runner-1", ProviderID: "attach-test", CurrentSession: "sess-1", CurrentChatID: "chat-1",
			},
		},
		turns:         stubTurnsForAttach{working: false},
		activity:      stubActivityForAttach{found: true},
		term:          &fakeTermForAttach{},
		chats:         chats,
		agents:        engineagents.New(),
		answers:       answerdesk.New(answerdesk.DefaultRetention, nil),
		inflightTurns: inflight.NewTurns(),
		work:          inflight.NewWork(),
		prompts:       agentjournal.NewPromptRequests(),
		home:          func() (string, error) { return t.TempDir(), nil },
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Socket: sockPath, Session: "sess-1", Cwd: "/work"},
	})
	return rs, chats
}
