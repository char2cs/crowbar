package runner

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// stopActivity records the stopped interruptions RecordStop writes — the only
// part of the activity ledger a Stop touches — and panics on anything else.
type stopActivity struct {
	agentactivity.EventStore

	mu      sync.Mutex
	stopped []string
}

func (a *stopActivity) Interrupt(_ context.Context, chatID, _, kind, _ string, _ time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if kind == engineagents.InterruptStopped {
		a.stopped = append(a.stopped, chatID)
	}
	return nil
}

func (a *stopActivity) ResolveInterruption(context.Context, string, string, string, string, time.Time) error {
	return nil
}

func (a *stopActivity) recorded() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.stopped...)
}

// realStopTurns is the real hook ingress over a recording activity ledger, so
// whether RecordStop fires is decided by production code.
func realStopTurns() (*turn.Turns, *stopActivity) {
	activity := &stopActivity{}
	return turn.New(turn.Deps{Activity: activity}), activity
}

// interruptRunner is runner-1, live on chat-1 over interrupt-test.
var interruptRunner = engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test", CurrentChatID: "chat-1"}

// retiring puts runner-1 live on chat-1 and fills in what its exit is
// recorded through.
func retiring(t *testing.T, rs *Runners) *Runners {
	t.Helper()
	rs.live = liveWith(t, interruptRunner)
	rs.surfaces = newSurfaceRegistry()
	rs.answers = answerdesk.New(answerdesk.DefaultRetention, nil)
	rs.activity = stubActivityForSpawn{}
	rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}}
	if rs.ledger == nil {
		rs.ledger = newTestLedger(t)
	}
	return rs
}

// runnerStarted records runner-1 started on chat-1, running a turn when
// running is set.
func runnerStarted(t *testing.T, rs *Runners, running bool) {
	t.Helper()
	rows := []ledger.Row{}
	started, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "runner-1", Provider: "interrupt-test"})
	require.NoError(t, err)
	rows = append(rows, started)
	if running {
		turnRow, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{RunnerID: "runner-1"})
		require.NoError(t, err)
		rows = append(rows, turnRow)
	}
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", rows...)
	require.NoError(t, err)
}

// retiredAs is the reason chat-1's ledger records runner-1's exit with; ""
// while none is recorded.
func retiredAs(t *testing.T, rs *Runners) string {
	t.Helper()
	exit, ok := chatState(t, rs.ledger).LastExit()
	if !ok {
		return ""
	}
	require.Equal(t, "runner-1", exit.RunnerID)
	return exit.Reason
}

// interruptServer is an api connection whose provider answers turn/interrupt
// only once release closes, as codex does once the turn truly ends.
func interruptServer(t *testing.T, asked chan<- struct{}, release <-chan struct{}) *apiconn {
	t.Helper()
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		close(asked)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		<-release
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		_ = conn.WriteMessage(websocket.TextMessage, resp)
		_, _, _ = conn.ReadMessage()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	driver, err := interruptTestAgent(t).StartAPIConn(ctx, sockPath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Close() })
	return &apiconn{driver: driver, ctx: ctx}
}

// Stop ends the turn in the ledger, records its divider, then asks the
// provider to interrupt and returns without waiting for the provider's answer.
func TestStopChat_EndsTheTurnThenInterruptsWithoutWaiting(t *testing.T) {
	asked, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	turns, activity := realStopTurns()
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		spawns:   inflight.NewGate(),
		turns:    turns,
	})
	runnerStarted(t, rs, true)
	rs.apiConns.set("runner-1", interruptServer(t, asked, release))

	require.NoError(t, rs.StopChat(t.Context(), "chat-1"))

	st := chatState(t, rs.ledger)
	assert.False(t, st.Busy(), "Stop ends the turn before it returns")
	assert.Equal(t, []string{"chat-1"}, activity.recorded())
	<-asked
	assert.Empty(t, retiredAs(t, rs), "an interruptible provider keeps its CLI")
}

// TestRegression_StopChatOnAnIdleChatActuallyRetires: there is nothing to
// interrupt on an idle chat, so Stop retires its runner.
func TestRegression_StopChatOnAnIdleChatActuallyRetires(t *testing.T) {
	asked := make(chan struct{})
	turns, activity := realStopTurns()
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		spawns:   inflight.NewGate(),
		turns:    turns,
		term:     &fakeTermForAttach{},
	})
	runnerStarted(t, rs, false)
	rs.apiConns.set("runner-1", interruptServer(t, asked, nil))

	require.NoError(t, rs.StopChat(t.Context(), "chat-1"))

	select {
	case <-asked:
		t.Fatal("StopChat sent turn/interrupt on an idle chat")
	default:
	}
	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs),
		"an idle chat's runner must actually be retired on close, not left running forever")
	assert.Empty(t, activity.recorded(), "closing an idle chat interrupts nothing")
}

// Closing a view ends the chat's session, even mid-turn on a provider that
// could interrupt in place: nothing on screen would watch the CLI.
func TestRegression_CloseChatRetiresARunnerMidTurnEvenWhenTheProviderCanInterrupt(t *testing.T) {
	asked := make(chan struct{})
	turns, activity := realStopTurns()
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		spawns:   inflight.NewGate(),
		turns:    turns,
		term:     &fakeTermForAttach{},
	})
	runnerStarted(t, rs, true)
	rs.apiConns.set("runner-1", interruptServer(t, asked, nil))

	require.NoError(t, rs.CloseChat(t.Context(), "chat-1"))

	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs), "a closed chat's runner must be retired, not interrupted in place")
	assert.Equal(t, []string{"chat-1"}, activity.recorded(), "the turn it cut short still gets its Stopped divider")
	assert.False(t, chatState(t, rs.ledger).Busy())
}

// A close whose HTTP request is abandoned (the app quit, the socket dropped)
// must still retire the runner: nothing else knows the view is gone.
func TestRegression_CloseChatRetiresEvenWhenTheRequestIsAbandoned(t *testing.T) {
	turns, _ := realStopTurns()
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{},
		spawns:   inflight.NewGate(),
		turns:    turns,
		term:     &fakeTermForAttach{},
	})
	runnerStarted(t, rs, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, rs.CloseChat(ctx, "chat-1"))
	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs), "an abandoned close request must not leave the runner live")
}

// quittingTerm is a PTY that takes its quit signal and exits later, on its own.
type quittingTerm struct{ fakeTermForAttach }

func (*quittingTerm) TerminateGraceful(context.Context, string) error { return nil }

// Stop on a provider that cannot interrupt tells its CLI to quit and returns
// without waiting: the CLI's own exit records the end.
func TestStopChat_RetiresWithoutWaitingForTheCLIToExit(t *testing.T) {
	turns, activity := realStopTurns()
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{},
		spawns:   inflight.NewGate(),
		turns:    turns,
		term:     &quittingTerm{},
	})
	pty := interruptRunner
	pty.TerminalSession = "pty-1"
	rs.live = liveWith(t, pty)
	runnerStarted(t, rs, true)

	stopped := make(chan error, 1)
	go func() { stopped <- rs.StopChat(t.Context(), "chat-1") }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop waited for the CLI to exit")
	}
	assert.False(t, chatState(t, rs.ledger).Busy(), "the turn ended before Stop returned")
	assert.Equal(t, []string{"chat-1"}, activity.recorded())
	assert.Empty(t, retiredAs(t, rs), "the CLI's exit is its own to record")

	rs.reconcileRunnerExit(t.Context(), "runner-1") // the PTY's exit callback
	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs))
}
