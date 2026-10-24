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

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// stubChatsForSpawn answers a create=false spawn's reads of the chat; the
// embedded nil interface panics on anything else.
type stubChatsForSpawn struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForSpawn) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

// SetProvider moves the chat's vendor; the one it has is refused, as the
// command refuses it.
func (s stubChatsForSpawn) SetProvider(_ context.Context, _, providerID string) (domain.Chat, error) {
	if providerID == s.chat.ProviderID {
		return s.chat, asynxModels.ErrValidation
	}
	return s.chat, nil
}

// spyLifecycle hears what the runner lifecycle announces, so a test blocks on
// a runner_started or runner_exited commit rather than on a timer.
type spyLifecycle struct {
	started chan string
	exited  chan string
}

func newSpyLifecycle() *spyLifecycle {
	return &spyLifecycle{started: make(chan string, 16), exited: make(chan string, 16)}
}

func (*spyLifecycle) Touch(context.Context, string) {}

func (s *spyLifecycle) ApplyRunner(_ context.Context, _, runnerID, kind string) {
	switch kind {
	case frameStarted:
		s.started <- runnerID
	case frameExited:
		s.exited <- runnerID
	}
}

// startedRow is the runner_started row chat-1's ledger holds for runnerID.
func startedRow(t *testing.T, l *ledgerstore.Store, runnerID string) ledger.RunnerStarted {
	t.Helper()
	rows, err := l.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	for _, r := range rows {
		if r.Kind != ledger.KindRunnerStarted {
			continue
		}
		body, err := ledger.DecodeBody[ledger.RunnerStarted](r)
		require.NoError(t, err)
		if body.RunnerID == runnerID {
			return body
		}
	}
	t.Fatalf("no runner_started row for %s", runnerID)
	return ledger.RunnerStarted{}
}

// spawnHarness is the smallest Runners that can drive spawnRunner end to end
// with a REAL shipped descriptor and a fake PTY seam.
type spawnHarness struct {
	rs        *Runners
	term      *fakeTermForAttach
	lifecycle *spyLifecycle
	home      string
	chatID    string
}

func newSpawnHarness(t *testing.T, surface string) *spawnHarness {
	t.Helper()
	// No vendor serve forks: a test that wants a connection seeds one.
	t.Setenv("CROWBAR_DISABLE_API_TRANSPORT", "1")

	home := t.TempDir()
	term := &fakeTermForAttach{}
	lifecycle := newSpyLifecycle()
	return &spawnHarness{
		rs: &Runners{
			agents:        engineagents.New(),
			ws:            stubWorkspaceForSpawn{home: home, worktree: home, chatsDir: home},
			providers:     stubProvidersForSpawn{},
			conversations: stubConversationsForSpawn{},
			chats:         stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", Surface: surface}},
			live:          newLiveTable(),
			snapshots:     lifecycle,
			term:          term,
			chatLock:      newTestLock(), parks: newParks(),
			apiConns: newAPIConnRegistry(),
			attached: newAttachRegistry(),
			surfaces: newSurfaceRegistry(),
			// The real registries a runner's death reconciles through.
			answers: answerdesk.New(answerdesk.DefaultRetention, nil),
			turns:   noopTurns{},
			ledger:  newTestLedger(t),
		},
		term: term, lifecycle: lifecycle, home: home, chatID: "chat-1",
	}
}

// seedLiveAPIConn registers a connection backed by a real process, as
// startAPIConn would: the runner's liveness hangs on that process.
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

// TestRegression_AnAPIDrivenSpawnForksNoOrphanPTY: an api-transport spawn whose
// provider is not hotswap must not also fork the descriptor's own TUI, which
// nothing drives and whose hooks still reach Crowbar.
func TestRegression_AnAPIDrivenSpawnForksNoOrphanPTY(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"an api connection that is driving the surface must not also fork the vendor's own TUI")
	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
	assert.Empty(t, in.TerminalSession,
		"a runner with no PTY must record no terminal session rather than a stale one")
}

// The connection is the runner once no PTY is forked, so its serve process
// dying reaches the reconcile a dead PTY would.
func TestRegression_APTYLessRunnerExitsWhenItsServeProcessDies(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	cmd := h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)
	<-h.lifecycle.started

	require.NoError(t, cmd.Process.Kill())

	select {
	case exited := <-h.lifecycle.exited:
		assert.Equal(t, "runner-1", exited)
	case <-time.After(10 * time.Second):
		t.Fatal("the serve process died and nothing ever exited the runner it was the whole of")
	}
}

// claude is hooks transport: its PTY IS the session, and nothing about the
// orphan fix may touch a single claude spawn.
func TestSpawnRunner_AHooksTransportProviderStillForksItsPTY(t *testing.T) {
	h := newSpawnHarness(t, "")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	require.Len(t, h.term.created, 1, "claude's PTY is the session; it must still be forked")
	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
	assert.Equal(t, h.term.created[0].termSessID, in.TerminalSession)
}

// An api-transport spawn whose connection never came up still forks its PTY:
// suppressing it would leave the chat with no process at all.
func TestSpawnRunner_AnAPITransportSpawnWithNoLiveConnectionStillForksItsPTY(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	require.Len(t, h.term.created, 1,
		"a failed api connection degrades to hooks over the PTY (design spec 2.2b), not to nothing")
}

// SwitchToTerminal's own teardown of a PTY-less runner's `serve` process is a
// handover, not the runner dying: the native view it forks is the runner now.
func TestRegression_SwitchToTerminalDoesNotExitThePTYLessRunnerItHandsOver(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	cmd := h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)
	<-h.lifecycle.started

	// What SwitchToTerminal does to the connection, in its own order.
	h.rs.handOverAPIConn("runner-1")
	h.rs.apiConns.drop("runner-1")
	waitProcessGone(t, cmd)

	select {
	case runnerID := <-h.lifecycle.exited:
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
// started with NO terminal session, whose native view is the only process it
// has left.
func ptylessRunners(t *testing.T) (*Runners, *spyLifecycle) {
	t.Helper()
	lifecycle := newSpyLifecycle()
	return &Runners{
		live:      liveWith(t, engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}),
		snapshots: lifecycle,
		apiConns:  newAPIConnRegistry(),
		attached:  newAttachRegistry(),
		surfaces:  newSurfaceRegistry(),
		chatLock:  newTestLock(), parks: newParks(),
		agents:     engineagents.New(),
		answers:    answerdesk.New(answerdesk.DefaultRetention, nil),
		turns:      noopTurns{},
		chats:      stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		ledger:     newTestLedger(t),
		background: newBackgroundWork(),
	}, lifecycle
}

// settled waits out every onAttachExit rs started, then reports any runner
// they exited.
func settled(rs *Runners, lifecycle *spyLifecycle) (string, bool) {
	rs.background.wg.Wait()
	select {
	case exited := <-lifecycle.exited:
		return exited, true
	default:
		return "", false
	}
}

// Retire and a provider switch kill the native view and re-establish nothing:
// for a runner with no PTY of its own that view was its last process.
func TestRegression_ANativeViewTornDownForGoodExitsThePTYLessRunner(t *testing.T) {
	rs, lifecycle := ptylessRunners(t)

	// An end's own order: mark the runner ending, forget the view, then kill it.
	rs.live.markExiting("runner-1", domain.AgentExitDisplaced)
	rs.attached.drop("runner-1")
	rs.onAttachExit("chat-1", "runner-1")(engineterminal.Exit{})

	select {
	case exited := <-lifecycle.exited:
		assert.Equal(t, "runner-1", exited)
	case <-time.After(5 * time.Second):
		t.Fatal("the native view died with nothing to replace it and the runner stayed live")
	}
}

// A view's exit leaves the runner when something else keeps it: its own PTY
// (the liveness authority), a re-established connection, or a detach, which
// ends nothing and re-establishes what replaces the view.
func TestOnAttachExit_LeavesARunnerSomethingElseKeeps(t *testing.T) {
	ending := func(rs *Runners) { rs.live.markExiting("runner-1", domain.AgentExitDisplaced) }
	for name, keep := range map[string]func(*Runners){
		"its own PTY": func(rs *Runners) {
			ending(rs)
			rs.live.update("runner-1", func(r *engineagents.Runner) { r.TerminalSession = "term-1" })
		},
		"a re-established connection": func(rs *Runners) { ending(rs); rs.apiConns.set("runner-1", &apiconn{}) },
		"a detach":                    func(*Runners) {},
	} {
		t.Run(name, func(t *testing.T) {
			rs, lifecycle := ptylessRunners(t)
			keep(rs)

			rs.attached.drop("runner-1")
			rs.onAttachExit("chat-1", "runner-1")(engineterminal.Exit{})

			exited, ok := settled(rs, lifecycle)
			assert.False(t, ok, "runner %s was exited while %s keeps it", exited, name)
		})
	}
}

// carrierTestDescriptor is api transport plus a restart_tui prompt_submit: an
// adopted spawn forks no PTY, so its argv carries nothing.
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
    delivery: stdio
    format: ndjson
    serve: [acme, serve]
    handshake: { call: initialize }
session: { resume: { arg: "resume {id}" }, persists: turn_start }
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

// seedAdoptableAPIConn registers on runner-1 what applyAPITransport leaves: a
// real driver with its session established, and a process for watchExit.
func (h *spawnHarness) seedAdoptableAPIConn(
	t *testing.T, stdio engineagents.APIStdio, agent engineagents.Agent,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	driver, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Close() })
	established, err := driver.EstablishSession(ctx, "prompt", map[string]string{"cwd": "/work"})
	require.NoError(t, err)
	require.NotEmpty(t, established["session_id"])

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	h.rs.apiConns.set("runner-1", &apiconn{
		serve: reapServe(cmd), driver: driver, ctx: ctx, cancel: cancel, agent: agent,
		tctx: engineagents.TemplateCtx{Session: established["session_id"], Cwd: "/work"},
	})
}

// promptDispatchServer answers the establish and the turn, publishing the
// turn's text; refuse makes it reject the turn.
func promptDispatchServer(t *testing.T, received chan<- string, refuse bool) engineagents.APIStdio {
	t.Helper()
	return fakeServe(t, func(conn *testutil.LinePeer) {
		for {
			// A failed read is the client closing: the test's own wait reports it.
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params struct {
					Text string `json:"text"`
				} `json:"params"`
			}
			if conn.ReadJSON(&req) != nil {
				return
			}
			if len(req.ID) == 0 {
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
			if conn.WriteJSON(body) != nil {
				return
			}
		}
	})
}

// An adopted spawn forks no PTY, so the connection it adopts is the only
// carrier its prompt has.
func TestRegression_AnAdoptedPTYLessSpawnStillDeliversThePromptItCarries(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	h.seedAdoptableAPIConn(t, stdio, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", spawnInput{
		runnerID: "runner-1", finalSteps: promptSteps, promptMessage: "hi there",
	}, false)
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

// A spawn that cannot get its prompt to a carrier fails, so the prompt is
// settled and the client keeps the user's text.
func TestRegression_AnAdoptedSpawnThatCannotDeliverItsPromptFailsTheSpawn(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, true)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	h.seedAdoptableAPIConn(t, stdio, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", spawnInput{
		runnerID: "runner-1", finalSteps: promptSteps, promptMessage: "hi there",
	}, false)

	require.ErrorIs(t, err, ErrSpawnPromptUndeliverable)
	select {
	case runnerID := <-h.lifecycle.started:
		t.Fatalf("runner %s was recorded live with the prompt it exists to deliver nowhere", runnerID)
	default:
	}
}

// An api-transport spawn whose connection never came up forks its PTY, and
// that PTY's argv carries the prompt.
func TestSpawnRunner_TheForkedPTYCarriesThePromptWhenNoConnectionCameUp(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", spawnInput{
		runnerID: "runner-1", finalSteps: promptSteps, promptMessage: "hi there",
	}, false)
	require.NoError(t, err)

	require.Len(t, h.term.created, 1)
	assert.Contains(t, h.term.created[0].argv, "hi there")
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

// TestRegression_ASpawnWithAChosenModelKeepsItsAPIConnection: a chosen model rides
// the serve process's own config channel (model.api_apply), so the chat keeps its
// api connection and the streaming it carries.
func TestRegression_ASpawnWithAChosenModelKeepsItsAPIConnection(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}
	h.seedAdoptableAPIConn(t, stdio, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a chosen model is declared on the api channel too; forking a PTY for it drops streaming")
	assert.True(t, h.rs.HasLiveAPIConnection("runner-1"))
}

// The record names the carrier that took the choice: selectionRequiresRestart
// compares against LaunchModel, so a model no carrier received sticks.
func TestRegression_ASelectionIsRecordedOnTheCarrierThatActuallyTookIt(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}
	h.seedAdoptableAPIConn(t, stdio, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
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

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	require.Len(t, h.term.created, 1)
	argv := h.term.created[0].argv
	assert.Contains(t, argv, "--model")
	assert.Contains(t, argv, "deep")
	assert.Contains(t, argv, `reasoning="high"`)

	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
	assert.Equal(t, "deep", in.LaunchModel)
	assert.NotEmpty(t, in.TerminalSession)
}

// A selection-carrying spawn forks no PTY either: its prompt rides the
// connection the choice does.
func TestRegression_ASelectionCarryingSpawnStillDeliversItsPromptOverTheConnection(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep"}}
	h.seedAdoptableAPIConn(t, stdio, agent)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", spawnInput{
		runnerID: "runner-1", finalSteps: promptSteps, promptMessage: "hi there",
	}, false)
	require.NoError(t, err)

	require.Empty(t, h.term.created)
	select {
	case got := <-received:
		assert.Equal(t, "hi there", got)
	case <-time.After(5 * time.Second):
		t.Fatal("the spawn forked no PTY and never delivered its prompt over the connection it adopted")
	}
}

// A carrier that renders nothing for a field reports it uncarried: recordRunner
// stamps what a carrier took, never what the chat intended.
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
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.seedAdoptableAPIConn(t, stdio, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a spawn with nothing to carry has nothing the adopted connection can drop")
}

// A permission level's api carrier is its vars:, rendered into the establish
// call rather than the serve argv; the shipped descriptor is that pairing.
func TestSpawnRunner_APermissionLevelAloneStillAdoptsTheConnection(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{PermissionLevel: "trusted"}}
	h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a permission level has a declared api carrier (its vars:), so it is not a reason to fork")
}

// An attaching spawn runs its attach argv in place of the plan: the connection
// it attaches to carries the prompt.
func TestSpawnRunner_AnAttachingSpawnDeliversItsPromptOverTheConnection(t *testing.T) {
	h := helperServeSpawn(t, "answer", true)
	asked := filepath.Join(t.TempDir(), "asked")
	t.Setenv(helperServeAsked, asked)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1", promptMessage: "hi"}, false)
	require.NoError(t, err)

	require.Len(t, h.term.created, 1, "the attach view forked")
	assert.Contains(t, askedCalls(t, asked), "turn/start hi")
}
