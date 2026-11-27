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

type stubChatsForSpawn struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForSpawn) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

func (s stubChatsForSpawn) SetProvider(_ context.Context, _, providerID string) (domain.Chat, error) {
	if providerID == s.chat.ProviderID {
		return s.chat, asynxModels.ErrValidation
	}
	return s.chat, nil
}

type spyLifecycle struct {
	started chan string
	exited  chan string
}

func newSpyLifecycle(t *testing.T) (*spyLifecycle, *ledgerstore.Store) {
	t.Helper()
	l := newTestLedger(t)
	s := &spyLifecycle{started: make(chan string, 16), exited: make(chan string, 16)}
	heard := map[ledger.Kind]chan string{ledger.KindRunnerStarted: s.started, ledger.KindRunnerExited: s.exited}
	l.Subscribe(func(_ string, rows []ledger.Row, _ ledger.State) {
		for _, r := range rows {
			if ch, ok := heard[r.Kind]; ok {
				body, _ := ledger.DecodeBody[ledger.RunnerExited](r)
				ch <- body.RunnerID
			}
		}
	})
	return s, l
}

func (*spyLifecycle) Touch(context.Context, string) {}

func startedRow(t *testing.T, l *ledgerstore.Store, runnerID string) ledger.RunnerStarted {
	t.Helper()
	rows, err := l.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	for _, r := range rows {
		if body, err := ledger.DecodeBody[ledger.RunnerStarted](r); r.Kind == ledger.KindRunnerStarted && err == nil && body.RunnerID == runnerID {
			return body
		}
	}
	t.Fatalf("no runner_started row for %s", runnerID)
	return ledger.RunnerStarted{}
}

type spawnHarness struct {
	rs        *Runners
	term      *fakeTermForAttach
	lifecycle *spyLifecycle
	home      string
	chatID    string
}

func newSpawnHarness(t *testing.T, surface string) *spawnHarness {
	t.Helper()
	home := t.TempDir()
	term := &fakeTermForAttach{}
	lifecycle, l := newSpyLifecycle(t)
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
			apiConns:       newAPIConnRegistry(),
			attached:       newAttachRegistry(),
			surfaces:       newSurfaceRegistry(),
			answers:        answerdesk.New(answerdesk.DefaultRetention, noLedger{}),
			turns:          noopTurns{},
			ledger:         l,
			noAPITransport: true,
		},
		term: term, lifecycle: lifecycle, home: home, chatID: "chat-1",
	}
}

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

func TestRegression_AnAPIDrivenSpawnForksNoOrphanPTY(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"an api connection that is driving the surface must not also fork the vendor's own TUI")
	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
	assert.Empty(t, in.TerminalSession,
		"a runner with no PTY must record no terminal session rather than a stale one")
}

func TestRegression_APTYLessRunnerExitsWhenItsServeProcessDies(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	cmd := h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
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

func TestSpawnRunner_AHooksTransportProviderStillForksItsPTY(t *testing.T) {
	h := newSpawnHarness(t, "")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	require.Len(t, h.term.created, 1, "claude's PTY is the session; it must still be forked")
	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
	assert.Equal(t, h.term.created[0].termSessID, in.TerminalSession)
}

func TestSpawnRunner_AnAPITransportSpawnWithNoLiveConnectionStillForksItsPTY(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	require.Len(t, h.term.created, 1,
		"a failed api connection degrades to hooks over the PTY (design spec 2.2b), not to nothing")
}

func TestRegression_SwitchToTerminalDoesNotExitThePTYLessRunnerItHandsOver(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	cmd := h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)
	<-h.lifecycle.started

	h.rs.handOverAPIConn("runner-1")
	h.rs.apiConns.drop("runner-1")
	waitProcessGone(t, cmd)

	select {
	case runnerID := <-h.lifecycle.exited:
		t.Fatalf("a handover exited runner %s; the native view it forked is the process now", runnerID)
	case <-time.After(300 * time.Millisecond):
	}
}

func waitProcessGone(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	require.Eventually(t, func() bool {
		return cmd.Process.Signal(syscall.Signal(0)) != nil
	}, 5*time.Second, 10*time.Millisecond, "the serve process never died")
}

func ptylessRunners(t *testing.T) (*Runners, *spyLifecycle) {
	t.Helper()
	lifecycle, l := newSpyLifecycle(t)
	return &Runners{
		live:      liveWith(t, engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}),
		snapshots: lifecycle,
		apiConns:  newAPIConnRegistry(),
		attached:  newAttachRegistry(),
		surfaces:  newSurfaceRegistry(),
		chatLock:  newTestLock(), parks: newParks(),
		agents:     engineagents.New(),
		answers:    answerdesk.New(answerdesk.DefaultRetention, noLedger{}),
		turns:      noopTurns{},
		chats:      stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		ledger:     l,
		background: newBackgroundWork(),
	}, lifecycle
}

func settled(rs *Runners, lifecycle *spyLifecycle) (string, bool) {
	rs.background.wg.Wait()
	select {
	case exited := <-lifecycle.exited:
		return exited, true
	default:
		return "", false
	}
}

func TestRegression_ANativeViewTornDownForGoodExitsThePTYLessRunner(t *testing.T) {
	rs, lifecycle := ptylessRunners(t)

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
    serve: [acme, serve]
    handshake: { call: initialize, notify: initialized }
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

func promptDispatchServer(t *testing.T, received chan<- string, refuse bool) engineagents.APIStdio {
	t.Helper()
	return fakeServe(t, func(conn *testutil.LinePeer) {
		for {
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
	})
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
	})

	require.ErrorIs(t, err, ErrSpawnPromptUndeliverable)
	select {
	case runnerID := <-h.lifecycle.started:
		t.Fatalf("runner %s was recorded live with the prompt it exists to deliver nowhere", runnerID)
	default:
	}
}

func TestSpawnRunner_TheForkedPTYCarriesThePromptWhenNoConnectionCameUp(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installCarrierDescriptor(t, h.home)
	promptSteps, err := agent.PromptSteps(false)
	require.NoError(t, err)

	_, err = h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test", spawnInput{
		runnerID: "runner-1", finalSteps: promptSteps, promptMessage: "hi there",
	})
	require.NoError(t, err)

	require.Len(t, h.term.created, 1)
	assert.Contains(t, h.term.created[0].argv, "hi there")
}

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

type selectionConversations struct {
	stubConversationsForSpawn
	sel engineagents.Selection
}

func (s selectionConversations) ChatSelection(
	context.Context, string,
) (engineagents.Selection, error) {
	return s.sel, nil
}

func TestRegression_ASpawnWithAChosenModelKeepsItsAPIConnection(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}
	h.seedAdoptableAPIConn(t, stdio, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a chosen model is declared on the api channel too; forking a PTY for it drops streaming")
	assert.True(t, h.rs.HasLiveAPIConnection("runner-1"))
}

func TestRegression_ASelectionIsRecordedOnTheCarrierThatActuallyTookIt(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}
	h.seedAdoptableAPIConn(t, stdio, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	in := startedRow(t, h.rs.ledger, <-h.lifecycle.started)
	assert.Equal(t, "deep", in.LaunchModel)
	assert.Equal(t, "high", in.LaunchEffort)
	assert.Empty(t, in.TerminalSession,
		"this runner IS its connection; the config channel is what carried the choice")
}

func TestSpawnRunner_TheForkedPTYCarriesTheSelectionWhenNoConnectionCameUp(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{Model: "deep", Effort: "high"}}

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"})
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
	})
	require.NoError(t, err)

	require.Empty(t, h.term.created)
	select {
	case got := <-received:
		assert.Equal(t, "hi there", got)
	case <-time.After(5 * time.Second):
		t.Fatal("the spawn forked no PTY and never delivered its prompt over the connection it adopted")
	}
}

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

func TestSpawnRunner_ASpawnWithNoSelectionStillAdoptsItsConnection(t *testing.T) {
	received := make(chan string, 1)
	stdio := promptDispatchServer(t, received, false)

	h := newSpawnHarness(t, engineagents.SurfaceChat)
	agent := installDescriptor(t, h.home, selectionCarrierDescriptor)
	h.seedAdoptableAPIConn(t, stdio, agent)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a spawn with nothing to carry has nothing the adopted connection can drop")
}

func TestSpawnRunner_APermissionLevelAloneStillAdoptsTheConnection(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.rs.conversations = selectionConversations{sel: engineagents.Selection{PermissionLevel: "trusted"}}
	h.seedLiveAPIConn(t, "runner-1")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.Empty(t, h.term.created,
		"a permission level has a declared api carrier (its vars:), so it is not a reason to fork")
}

func TestSpawnRunner_AnAttachingSpawnDeliversItsPromptOverTheConnection(t *testing.T) {
	h := helperServeSpawn(t, "answer", true)
	asked := filepath.Join(t.TempDir(), "asked")
	t.Setenv(helperServeAsked, asked)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-1", promptMessage: "hi"})
	require.NoError(t, err)

	require.Len(t, h.term.created, 1, "the attach view forked")
	assert.Contains(t, askedCalls(t, asked), "turn/start hi")
}
