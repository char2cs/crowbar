package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubChatsForSpawn answers the one read a create=false spawn makes of the
// chat aggregate — GetChat, for storedSurface. The embedded nil interface
// panics on anything else.
type stubChatsForSpawn struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForSpawn) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

func (s stubChatsForSpawn) AbandonTurn(
	context.Context, string, time.Time,
) (domain.Chat, error) {
	return s.chat, nil
}

// SetProvider is the durable-vendor restatement every committed spawn makes
// (recordChatProvider). Best-effort there, so the value is never read back here.
func (s stubChatsForSpawn) SetProvider(
	context.Context, string, string,
) (domain.Chat, error) {
	return s.chat, nil
}

// stubActivityForSpawn answers the one write closeAbandonedTurn makes of the
// conversation record.
type stubActivityForSpawn struct {
	agentactivity.EventStore
}

func (stubActivityForSpawn) Abandon(context.Context, string, time.Time) error { return nil }

// spyRunnerStoreForSpawn records the Start input (the only place a runner's
// terminal-session identity is ever written) and signals every Exit on a
// channel, so a test can block on the REAL reconcile rather than on a timer.
type spyRunnerStoreForSpawn struct {
	agentrunner.EventStore
	started chan agentrunner.StartInput
	exited  chan string
	runner  engineagents.Runner
	gone    bool
}

func newSpyRunnerStoreForSpawn() *spyRunnerStoreForSpawn {
	return &spyRunnerStoreForSpawn{
		started: make(chan agentrunner.StartInput, 4),
		exited:  make(chan string, 4),
	}
}

func (s *spyRunnerStoreForSpawn) Start(
	_ context.Context, in agentrunner.StartInput,
) (engineagents.Runner, error) {
	s.runner = engineagents.Runner{
		ID: in.RunnerID, ProviderID: in.ProviderID, CurrentChatID: in.ChatID,
		WorkspaceID: in.WorkspaceID, TerminalSession: in.TerminalSession,
	}
	s.started <- in
	return s.runner, nil
}

func (s *spyRunnerStoreForSpawn) LiveRunnersForChat(
	context.Context, string,
) ([]engineagents.Runner, error) {
	return nil, nil
}

func (s *spyRunnerStoreForSpawn) Get(
	_ context.Context, runnerID string,
) (engineagents.Runner, error) {
	if s.gone || s.runner.ID != runnerID {
		return engineagents.Runner{}, agentrunner.ErrNotFound
	}
	return s.runner, nil
}

func (s *spyRunnerStoreForSpawn) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	return engineagents.Runner{}, agentrunner.ErrNotFound
}

func (s *spyRunnerStoreForSpawn) Exit(
	_ context.Context, runnerID string, _ time.Time,
) (engineagents.Runner, error) {
	s.gone = true
	s.exited <- runnerID
	return s.runner, nil
}

// spawnHarness is the smallest Runners that can drive spawnRunner end to end
// with a REAL shipped descriptor and a fake PTY seam.
type spawnHarness struct {
	rs     *Runners
	term   *fakeTermForAttach
	store  *spyRunnerStoreForSpawn
	home   string
	chatID string
}

func newSpawnHarness(t *testing.T, surface string) *spawnHarness {
	t.Helper()
	// The api transport is switched OFF so startAPIConn forks no real vendor
	// process; a test that wants a live connection seeds the registry itself.
	t.Setenv("CROWBAR_DISABLE_API_TRANSPORT", "1")

	home := t.TempDir()
	term := &fakeTermForAttach{}
	store := newSpyRunnerStoreForSpawn()
	return &spawnHarness{
		rs: &Runners{
			agents:        engineagents.New(),
			ws:            stubWorkspaceForSpawn{home: home, worktree: home, chatsDir: home},
			providers:     stubProvidersForSpawn{},
			conversations: stubConversationsForSpawn{},
			chats:         stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", Surface: surface}},
			runnerStore:   store,
			term:          term,
			pendingHooks:  inflight.NewHooks(),
			apiConns:      newAPIConnRegistry(),
			attached:      newAttachRegistry(),
			surfaces:      newSurfaceRegistry(),
			// Everything reconcileRunnerExit touches on its way through a
			// runner's death — the real registries, so the exit path under
			// test is the production one and not a nil-guarded shortcut.
			answers:       answerdesk.New(answerdesk.DefaultRetention, nil),
			inflightTurns: inflight.NewTurns(),
			work:          inflight.NewWork(),
			turns:         noopTurns{},
			activity:      stubActivityForSpawn{},
			prompts:       agentjournal.NewPromptRequests(),
			home:          func() (string, error) { return home, nil },
		},
		term: term, store: store, home: home, chatID: "chat-1",
	}
}

// seedLiveAPIConn registers a connection backed by a REAL background process,
// exactly as startAPIConn would — the process is what the runner's liveness
// now hangs on, so a fake one would prove nothing.
func (h *spawnHarness) seedLiveAPIConn(t *testing.T, runnerID string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.rs.apiConns.set(runnerID, &apiconn{serve: reapServe(cmd), ctx: ctx, cancel: cancel})
	return cmd
}

// TestRegression_AnAPIDrivenSpawnForksNoOrphanPTY is the reported defect:
// EVERY api-transport codex spawn also forked a full interactive `codex` TUI
// that nothing drove. attach is only rendered for a HOTSWAP provider
// (applyAPITransport), codex is not one, so pointPlanAtAttach no-opped and
// forkCLI forked the descriptor's own spawn.cmd instead. Measured live: 25
// bare codex TUIs against 3 chats. Each one fires codex's whole hooks config
// into Crowbar, which is what let a user type into an unrelated session and
// have it promoted into a new chat.
func TestRegression_AnAPIDrivenSpawnForksNoOrphanPTY(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"an api connection that is driving the surface must not also fork the vendor's own TUI")
	in := <-h.store.started
	assert.Empty(t, in.TerminalSession,
		"a runner with no PTY must record no terminal session rather than a stale one")
}

// The connection IS the runner once no PTY is forked, so the serve process
// dying has to reach the exact reconcile a dead PTY used to — a runner that
// can never be observed to exit is worse than the orphan it replaced.
func TestRegression_APTYLessRunnerExitsWhenItsServeProcessDies(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	cmd := h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)
	<-h.store.started

	require.NoError(t, cmd.Process.Kill())

	select {
	case exited := <-h.store.exited:
		assert.Equal(t, "runner-1", exited)
	case <-time.After(10 * time.Second):
		t.Fatal("the serve process died and nothing ever exited the runner it was the whole of")
	}
}

// claude is hooks transport: its PTY IS the session, and nothing about the
// orphan fix may touch a single claude spawn.
func TestSpawnRunner_AHooksTransportProviderStillForksItsPTY(t *testing.T) {
	h := newSpawnHarness(t, "")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	require.Len(t, h.term.created, 1, "claude's PTY is the session; it must still be forked")
	in := <-h.store.started
	assert.Equal(t, h.term.created[0].termSessID, in.TerminalSession)
}

// An api-transport spawn whose connection never came up has nothing else to
// be: the PTY is the only codex there is, and suppressing it would leave the
// chat with no process at all.
func TestSpawnRunner_AnAPITransportSpawnWithNoLiveConnectionStillForksItsPTY(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	require.Len(t, h.term.created, 1,
		"a failed api connection degrades to hooks over the PTY (design spec 2.2b), not to nothing")
}

// TestRegression_SwitchToTerminalDoesNotExitThePTYLessRunnerItHandsOver is
// the hole the orphan-PTY fix opened in the other direction: with the `serve`
// process now carrying the runner's liveness, SwitchToTerminal's own
// deliberate teardown of that connection (attach.go — it MUST go first, codex
// allows one writer per thread) looked exactly like the process dying, and
// reconciled the runner away underneath the native view it had just forked.
func TestRegression_SwitchToTerminalDoesNotExitThePTYLessRunnerItHandsOver(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	cmd := h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)
	<-h.store.started

	// What SwitchToTerminal does to the connection, in its own order.
	h.rs.handOverAPIConn("runner-1")
	h.rs.apiConns.drop("runner-1")
	waitProcessGone(t, cmd)

	select {
	case runnerID := <-h.store.exited:
		t.Fatalf("a handover exited runner %s; the native view it forked is the process now", runnerID)
	case <-time.After(300 * time.Millisecond):
	}
}

// waitProcessGone blocks on the process actually being reaped, so the
// assertion after it is about the exit path and never about timing.
func waitProcessGone(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	require.Eventually(t, func() bool {
		return cmd.Process.Signal(syscall.Signal(0)) != nil
	}, 5*time.Second, 10*time.Millisecond, "the serve process never died")
}

// ptylessRunners is the teardown half of the harness: a runner already
// recorded with NO terminal session, whose native view is the only process
// it has left.
func ptylessRunners(store *spyRunnerStoreForSpawn) *Runners {
	store.runner = engineagents.Runner{
		ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1", TerminalSession: "",
	}
	return &Runners{
		runnerStore:   store,
		apiConns:      newAPIConnRegistry(),
		attached:      newAttachRegistry(),
		surfaces:      newSurfaceRegistry(),
		spawns:        inflight.NewGate(),
		agents:        engineagents.New(),
		answers:       answerdesk.New(answerdesk.DefaultRetention, nil),
		inflightTurns: inflight.NewTurns(),
		work:          inflight.NewWork(),
		turns:         noopTurns{},
		activity:      stubActivityForSpawn{},
		chats:         stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		prompts:       agentjournal.NewPromptRequests(),
		home:          func() (string, error) { return "", nil },
	}
}

// TestRegression_ANativeViewTornDownForGoodExitsThePTYLessRunner: retire and
// a provider switch drop rs.attached and kill the native view without ever
// re-establishing anything. For a runner with no PTY of its own that view WAS
// its last process — and with the companion PTY gone there is nothing else
// left whose death could carry the row away, so the chat would advertise a
// live agent forever.
func TestRegression_ANativeViewTornDownForGoodExitsThePTYLessRunner(t *testing.T) {
	store := newSpyRunnerStoreForSpawn()
	rs := ptylessRunners(store)

	// retire()'s own order: forget the view first, then kill it.
	rs.attached.drop("runner-1")
	rs.onAttachExit("chat-1", "runner-1")()

	select {
	case exited := <-store.exited:
		assert.Equal(t, "runner-1", exited)
	case <-time.After(5 * time.Second):
		t.Fatal("the native view died with nothing to replace it and the runner row stayed live")
	}
}

// The same teardown against a runner that still HAS its own PTY must change
// nothing: that PTY is the liveness authority, and its own exit callback is
// what ends the runner.
func TestOnAttachExit_ARunnerWithItsOwnPTYIsLeftToThatPTY(t *testing.T) {
	store := newSpyRunnerStoreForSpawn()
	rs := ptylessRunners(store)
	store.runner.TerminalSession = "term-1"

	rs.attached.drop("runner-1")
	rs.onAttachExit("chat-1", "runner-1")()

	select {
	case exited := <-store.exited:
		t.Fatalf("runner %s was exited while its own PTY is still the authority on that", exited)
	case <-time.After(200 * time.Millisecond):
	}
}

// A re-established connection is a process again, so the view it replaced
// dying must not end the runner.
func TestOnAttachExit_AReestablishedConnectionKeepsThePTYLessRunnerAlive(t *testing.T) {
	store := newSpyRunnerStoreForSpawn()
	rs := ptylessRunners(store)
	rs.apiConns.set("runner-1", &apiconn{})

	rs.attached.drop("runner-1")
	rs.onAttachExit("chat-1", "runner-1")()

	select {
	case exited := <-store.exited:
		t.Fatalf("runner %s was exited while its api connection is live", exited)
	case <-time.After(200 * time.Millisecond):
	}
}

// carrierTestDescriptor is the MIXED shape the live defect needs, and the one
// every shipped api-transport provider is in: `transport: api` (so a spawn
// whose connection comes up forks no PTY at all — apiConnIsTheRunner) PLUS
// presentation.prompt_submit `strategy: restart_tui`, which renders the user's
// message into that PTY's own argv. Those two together are what let a
// prompt-bearing spawn complete with nothing carrying the prompt.
const carrierTestDescriptor = `
id: carrier-test
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
    serve: [acme, serve]
    handshake: { call: initialize }
session:
  resume: { arg: "resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
`

func installCarrierDescriptor(t *testing.T, home string) engineagents.Agent {
	t.Helper()
	return installDescriptor(t, home, carrierTestDescriptor)
}

func installDescriptor(t *testing.T, home, body string) engineagents.Agent {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "carrier-test.yaml"), []byte(body), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "carrier-test")
	require.NoError(t, err)
	return a
}

// seedAdoptableAPIConn registers exactly what a successful applyAPITransport
// leaves behind: a REAL driver (over an in-process server, so no vendor
// subprocess is forked) whose session has already been ESTABLISHED, with the
// confirmed id written back into tctx the way applyAPITransport writes it —
// plus a real background process for watchExit to arm against.
func (h *spawnHarness) seedAdoptableAPIConn(
	t *testing.T, runnerID, sockPath string, agent engineagents.Agent,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	driver, err := agent.StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Close() })
	established, err := driver.EstablishSession(ctx, "prompt", map[string]string{"cwd": "/work"})
	require.NoError(t, err)
	require.NotEmpty(t, established["session_id"])

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	h.rs.apiConns.set(runnerID, &apiconn{
		serve: reapServe(cmd), driver: driver, ctx: ctx, cancel: cancel, agent: agent,
		tctx: engineagents.TemplateCtx{Session: established["session_id"], Cwd: "/work"},
	})
}

// promptDispatchServer answers the two calls this descriptor's prompt event
// makes — the session establish, then the turn — publishing the turn's text so
// a test can block on the REAL wire signal rather than on a timer. refuse makes
// it reject the turn the way a CLI that cannot take one does.
func promptDispatchServer(t *testing.T, received chan<- string, refuse bool) string {
	t.Helper()
	return fakeWSServer(t, func(conn *websocket.Conn) {
		for {
			// A read that fails means the client closed: return quietly and let
			// the test's own wait report it, rather than failing from this
			// goroutine with a teardown-shaped error.
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params struct {
					Text string `json:"text"`
				} `json:"params"`
			}
			if json.Unmarshal(msg, &req) != nil || len(req.ID) == 0 {
				continue
			}
			body := map[string]any{"id": req.ID, "result": map[string]any{}}
			switch req.Method {
			case "thread/start":
				body["result"] = map[string]any{"thread": map[string]any{"id": "sess-1"}}
			case "turn/start":
				received <- req.Params.Text
				if refuse {
					delete(body, "result")
					body["error"] = map[string]any{"code": 1, "message": "boom: the CLI refused the turn"}
				}
			}
			resp, _ := json.Marshal(body)
			if conn.WriteMessage(websocket.TextMessage, resp) != nil {
				return
			}
		}
	})
}

// TestRegression_AnAdoptedPTYLessSpawnStillDeliversThePromptItCarries is the
// reported data loss: a codex chat answered a handoff prompt with nothing at
// all, no reply and not even a user bubble, and its prompt journal sat on
// `spawned` forever. An api-transport provider delivers a restart_tui prompt
// in the forked PTY's argv — and adoptAPIConn forks NOTHING, so the whole
// plan, prompt included, was built and thrown away while the spawn reported
// success. The connection it adopts instead is the only carrier that exists.
func TestRegression_AnAdoptedPTYLessSpawnStillDeliversThePromptItCarries(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, promptSteps, "", 0, false, "", false, "hi there")
	require.NoError(t, err)

	require.Empty(t, h.term.created,
		"an adopted runner forks no PTY, so no argv can be carrying this prompt")
	select {
	case got := <-received:
		assert.Equal(t, "hi there", got)
	case <-time.After(5 * time.Second):
		t.Fatal("the spawn carried a prompt, forked no PTY to put it in, and never delivered it over the connection it adopted instead")
	}
}

// TestRegression_AnAdoptedSpawnThatCannotDeliverItsPromptFailsTheSpawn is the
// other half of the invariant: a spawn that carries a prompt and cannot get it
// to a carrier must FAIL, so the delivery journal settles as uncertain and the
// client keeps the user's text. Reporting success with the message nowhere is
// the shape this whole change exists to make unreachable.
func TestRegression_AnAdoptedSpawnThatCannotDeliverItsPromptFailsTheSpawn(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, true)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, promptSteps, "", 0, false, "", false, "hi there")

	require.ErrorIs(t, err, ErrSpawnPromptUndeliverable)
	select {
	case in := <-h.store.started:
		t.Fatalf("runner %s was recorded live with the prompt it exists to deliver nowhere", in.RunnerID)
	default:
	}
}

// The degrade branch, pinned: an api-transport spawn whose connection never
// came up is the ONE case that still forks the vendor's own PTY (design spec
// 2.2b), and that PTY's argv is where the prompt rides. Nothing about the
// adopted-carrier path above may take this away.
func TestSpawnRunner_TheForkedPTYCarriesThePromptWhenNoConnectionCameUp(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, promptSteps, "", 0, false, "", false, "hi there")
	require.NoError(t, err)

	require.Len(t, h.term.created, 1)
	assert.Contains(t, h.term.created[0].argv, "hi there")
}

// TestRegression_AnAdoptedSpawnsDeliveryJournalCanReachAccepted is the user's
// own evidence, turned into an assertion: their journal held
// `spawned | runner=<the adopted one>` forever, because MarkSpawned records a
// delivery the provider was never actually given and only the provider's own
// acknowledgement of that text can advance it.
//
// It drives the TAIL of submitPromptLocked verbatim — Begin, spawnRunner with
// the preallocated replacement id, commitPromptSpawn — against the real
// journal, and then acknowledges exactly the text that came off the wire, the
// way the user_prompt echo does. Before the fix nothing reaches the wire at
// all, so there is no text to acknowledge and the record is stuck.
func TestRegression_AnAdoptedSpawnsDeliveryJournalCanReachAccepted(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	const text = "hi there"
	journalDir, err := h.rs.promptJournalDirFor(h.chatID)
	require.NoError(t, err)
	requestID := uuid.NewString()
	textHash := agentjournal.PromptTextHash(text)
	_, existing, err := h.rs.prompts.Begin(
		journalDir, requestID, text, textHash, "carrier-test", "outgoing-runner", "runner-1", time.Now(),
	)
	require.NoError(t, err)
	require.False(t, existing)

	runnerID, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, promptSteps, "", 0, false, "", false, text)
	require.NoError(t, err)
	_, err = h.rs.commitPromptSpawn(context.Background(), journalDir, requestID, textHash, runnerID)
	require.NoError(t, err)

	record, found, err := h.rs.prompts.Lookup(journalDir, requestID, textHash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, agentjournal.PromptStateSpawned, record.State)

	var delivered string
	select {
	case delivered = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing carried this delivery, so no acknowledgement can ever advance its journal past spawned")
	}
	// The provider's own acknowledgement of what it received — the hook/event
	// echo, which is the ONLY thing that advances this record.
	require.NoError(t, h.rs.ConfirmPromptAccepted(context.Background(),
		domain.Chat{ID: h.chatID}, engineagents.Runner{ID: runnerID, ProviderID: "carrier-test"}, delivered))

	record, found, err = h.rs.prompts.Lookup(journalDir, requestID, textHash)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateAccepted, record.State)
}

// selectionCarrierDescriptor is carrierTestDescriptor plus the OTHER thing a
// spawn carries: a model/effort choice, declared on BOTH carriers — apply:
// for the argv of a forked PTY, api_apply: for the config of an adopted
// connection. An api-transport descriptor may not declare only the first
// (rules.selectionAPICarrier): that spawn can fork nothing to put it on.
const selectionCarrierDescriptor = carrierTestDescriptor + `
model:
  available: [fast, deep]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
  api_apply:
    - pass_arg: { arg: "-c", value: 'model="{model}"' }
effort:
  available: { "*": [low, high] }
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "-c", value: 'reasoning="{effort}"' }
  api_apply:
    - pass_arg: { arg: "-c", value: 'reasoning="{effort}"' }
`

// selectionConversations pins the chat's model/effort choice — the one read
// spawnPreflight makes that decides what this spawn has to carry.
type selectionConversations struct {
	stubConversationsForSpawn
	sel engineagents.Selection
}

func (s selectionConversations) ChatSelection(
	context.Context, string, bool,
) (engineagents.Selection, error) {
	return s.sel, nil
}

// TestRegression_ASpawnWithAChosenModelKeepsItsAPIConnection is the reported
// defect, and the reversal of the workaround that first answered it. A codex
// chat with an explicit model was made to skip its api connection and fork a
// PTY, because the selection was believed to have no api carrier — which cost
// that chat streaming deltas and reasoning summaries for the life of the chat.
//
// It does have one: the serve process's own config channel (model.api_apply),
// verified against codex-cli 0.154.0, whose `app-server --help` documents
// `-c model="o3"` and offers no --model at all. So the connection stays.
func TestRegression_ASpawnWithAChosenModelKeepsItsAPIConnection(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a chosen model is declared on the api channel too; forking a PTY for it drops streaming")
	assert.True(t, h.rs.HasLiveAPIConnection("runner-1"))
}

// The record must describe the carrier that actually took the choice.
// LaunchModel is the only authority on what a live CLI is running —
// selectionRequiresRestart compares against it — so a row claiming a model no
// carrier received is what makes a revert permanent rather than wrong once.
func TestRegression_ASelectionIsRecordedOnTheCarrierThatActuallyTookIt(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	in := <-h.store.started
	assert.Equal(t, "deep", in.LaunchModel)
	assert.Equal(t, "high", in.LaunchEffort)
	assert.Empty(t, in.TerminalSession,
		"this runner IS its connection; the config channel is what carried the choice")
}

// The degrade branch's half of the same rule: no connection came up, so the
// forked PTY is the carrier and its own argv is where the choice rides.
func TestSpawnRunner_TheForkedPTYCarriesTheSelectionWhenNoConnectionCameUp(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	require.Len(t, h.term.created, 1)
	argv := h.term.created[0].argv
	assert.Contains(t, argv, "--model")
	assert.Contains(t, argv, "deep")
	assert.Contains(t, argv, `reasoning="high"`)

	in := <-h.store.started
	assert.Equal(t, "deep", in.LaunchModel)
	assert.NotEmpty(t, in.TerminalSession)
}

// A selection-carrying spawn is an ordinary adopted spawn in every other way:
// it forks no PTY, so its PROMPT rides the same connection the choice does.
// The old workaround forked one and put both in its argv, which is exactly
// the streaming loss this reverses.
func TestRegression_ASelectionCarryingSpawnStillDeliversItsPromptOverTheConnection(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep"}}
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, promptSteps, "", 0, false, "", false, "hi there")
	require.NoError(t, err)

	require.Empty(t, h.term.created)
	select {
	case got := <-received:
		assert.Equal(t, "hi there", got)
	case <-time.After(5 * time.Second):
		t.Fatal("the spawn forked no PTY and never delivered its prompt over the connection it adopted")
	}
}

// carriedSelection is what keeps the record from lying: recordRunner stamps
// what a carrier TOOK, never what the chat intended. A carrier that renders
// nothing for a field must report that field as uncarried, or
// selectionRequiresRestart compares equal forever and the revert is permanent.
func TestCarriedSelection_ReportsOnlyTheFieldsTheCarrierRenders(t *testing.T) {
	sel := engineagents.Selection{Model: "deep", Effort: "high", PermissionLevel: "trusted"}
	modelOnly := func(s engineagents.Selection) []engineagents.InjectStep {
		if s.Model == "" {
			return nil
		}
		return []engineagents.InjectStep{{Verb: "pass_arg"}}
	}

	assert.Equal(t,
		engineagents.Selection{Model: "deep", PermissionLevel: "trusted"},
		carriedSelection(sel, modelOnly),
		"effort reached no carrier, so nothing may record it as launched")

	none := func(engineagents.Selection) []engineagents.InjectStep { return nil }
	assert.Equal(t, engineagents.Selection{PermissionLevel: "trusted"}, carriedSelection(sel, none))
}

// A spawn that carries no selection at all still adopts — nothing about a
// chat that chose nothing changes.
func TestSpawnRunner_ASpawnWithNoSelectionStillAdoptsItsConnection(t *testing.T) {
	received := make(chan string, 1)
	sockPath := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.seedAdoptableAPIConn(t, "runner-1", sockPath, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a spawn with nothing to carry has nothing the adopted connection can drop")
}

// A permission level's own api carrier is a different shape from model's and
// effort's: it declares vars:, which applyAPITransport renders into the
// establish call's send: tree rather than onto the serve argv. Driven against
// the REAL shipped descriptor, the only place that pairing exists.
func TestSpawnRunner_APermissionLevelAloneStillAdoptsTheConnection(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{PermissionLevel: "trusted"}}
	h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a permission level has a declared api carrier (its vars:), so it is not a reason to fork")
}
