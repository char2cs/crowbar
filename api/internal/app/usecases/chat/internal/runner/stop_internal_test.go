package runner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func stopsRecorded(t *testing.T, rs *Runners) []string {
	t.Helper()
	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	var out []string
	for _, r := range rows {
		if r.Kind != ledger.KindMarker {
			continue
		}
		body, err := ledger.DecodeBody[ledger.Marker](r)
		require.NoError(t, err)
		if body.Kind == ledger.MarkerStopped {
			out = append(out, "chat-1")
		}
	}
	return out
}

var interruptRunner = engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "interrupt-test", CurrentChatID: "chat-1"}

func retiring(t *testing.T, rs *Runners) *Runners {
	t.Helper()
	rs.live = liveWith(t, interruptRunner)
	rs.surfaces = newSurfaceRegistry()
	rs.answers = answerdesk.New(answerdesk.DefaultRetention, noLedger{})
	rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}}
	if rs.ledger == nil {
		rs.ledger = newTestLedger(t)
	}
	if rs.chatLock == nil {
		rs.chatLock = newTestLock()
	}
	return rs
}

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

func retiredAs(t *testing.T, rs *Runners) string {
	t.Helper()
	exit, ok := chatState(t, rs.ledger).LastExit()
	if !ok {
		return ""
	}
	require.Equal(t, "runner-1", exit.RunnerID)
	return exit.Reason
}

func interruptServer(t *testing.T, asked chan<- struct{}, release <-chan struct{}) *apiconn {
	t.Helper()
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		msg, err := conn.Read()
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
		_ = conn.Write(resp)
		_, _ = conn.Read()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	driver, err := interruptTestAgent(t).StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Close() })
	return &apiconn{driver: driver, ctx: ctx}
}

func (noopTurns) LiveDiff(string, string) string { return "" }

type diffTurns struct {
	noopTurns
	diff string
}

func (d diffTurns) LiveDiff(string, string) string { return d.diff }

func TestStopChat_EndsTheTurnThenInterruptsWithoutWaiting(t *testing.T) {
	asked, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		chatLock: newTestLock(), parks: newParks(),
		turns: diffTurns{diff: "+x"},
	})
	runnerStarted(t, rs, true)
	rs.apiConns.set("runner-1", interruptServer(t, asked, release))

	require.NoError(t, rs.StopChat(t.Context(), "chat-1"))

	st := chatState(t, rs.ledger)
	assert.False(t, st.Busy(), "Stop ends the turn before it returns")
	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	var diffs []string
	for _, r := range rows {
		if end, err := ledger.DecodeBody[ledger.TurnEnded](r); err == nil && r.Kind == ledger.KindTurnEnded {
			diffs = append(diffs, end.Diff)
		}
	}
	assert.Equal(t, []string{"+x"}, diffs, "the stop keeps the turn's diff")
	assert.Equal(t, []string{"chat-1"}, stopsRecorded(t, rs))
	<-asked
	assert.Empty(t, retiredAs(t, rs), "an interruptible provider keeps its CLI")
}

func TestRegression_StopChatOnAnIdleChatActuallyRetires(t *testing.T) {
	asked := make(chan struct{})
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
		term:  &fakeTermForAttach{},
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
	assert.Empty(t, stopsRecorded(t, rs), "closing an idle chat interrupts nothing")
}

func TestRegression_KillChatRetiresARunnerMidTurnEvenWhenTheProviderCanInterrupt(t *testing.T) {
	asked := make(chan struct{})
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
		term:  &fakeTermForAttach{},
	})
	runnerStarted(t, rs, true)
	rs.apiConns.set("runner-1", interruptServer(t, asked, nil))

	require.NoError(t, rs.KillChat(t.Context(), "chat-1"))

	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs), "a killed chat's runner must be retired, not interrupted in place")
	assert.Equal(t, []string{"chat-1"}, stopsRecorded(t, rs), "the turn it cut short still gets its Stopped divider")
	assert.False(t, chatState(t, rs.ledger).Busy())
}

func TestRegression_KillChatRetiresEvenWhenTheRequestIsAbandoned(t *testing.T) {
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{},
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
		term:  &fakeTermForAttach{},
	})
	runnerStarted(t, rs, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, rs.KillChat(ctx, "chat-1"))
	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs), "an abandoned kill request must not leave the runner live")
}

type quittingTerm struct{ fakeTermForAttach }

func (*quittingTerm) TerminateGraceful(context.Context, string) error { return nil }

func TestStopChat_RetiresWithoutWaitingForTheCLIToExit(t *testing.T) {
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{},
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
		term:  &quittingTerm{},
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
	assert.Equal(t, []string{"chat-1"}, stopsRecorded(t, rs))
	assert.Empty(t, retiredAs(t, rs), "the CLI's exit is its own to record")

	rs.reconcileRunnerExit(t.Context(), "runner-1")
	assert.Equal(t, domain.AgentExitStopped, retiredAs(t, rs))
}

func TestRegression_StopChatRecordsAfterAHookHoldingTheChat(t *testing.T) {
	asked, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
	})
	runnerStarted(t, rs, true)
	rs.apiConns.set("runner-1", interruptServer(t, asked, release))
	unlock, err := rs.chatLock.Lock(t.Context(), "chat-1")
	require.NoError(t, err)

	stopped := make(chan error, 1)
	go func() { stopped <- rs.StopChat(t.Context(), "chat-1") }()
	hook, err := ledger.NewRow(ledger.KindAssistantText, "T-1", "", ledger.AssistantText{MessageID: "m1", Text: "said"})
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", hook)
	require.NoError(t, err)
	unlock()

	require.NoError(t, <-stopped)
	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	kinds := make([]ledger.Kind, 0, len(rows))
	for _, r := range rows {
		kinds = append(kinds, r.Kind)
	}
	assert.Equal(t, ledger.KindMarker, kinds[len(kinds)-1], "the divider is the last row")
	assert.Contains(t, kinds[:len(kinds)-1], ledger.KindAssistantText, "the hook's row precedes it")
}

func TestRegression_StopChatRecordsOnlyTheStoppedReason(t *testing.T) {
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{},
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
		term:  &fakeTermForAttach{},
	})
	runnerStarted(t, rs, false)

	require.NoError(t, rs.StopChat(t.Context(), "chat-1"))

	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	var reasons []string
	for _, r := range rows {
		if exit, err := ledger.DecodeBody[ledger.RunnerExited](r); err == nil && r.Kind == ledger.KindRunnerExited {
			reasons = append(reasons, exit.Reason)
		}
	}
	assert.Equal(t, []string{domain.AgentExitStopped}, reasons)
}

func TestStopChat_ClearsAWorkingChatWhoseRunnerIsGone(t *testing.T) {
	rs := retiring(t, &Runners{chatLock: newTestLock(), parks: newParks(), turns: noopTurns{}})
	rs.live = liveWith(t)
	runnerStarted(t, rs, true)
	require.True(t, chatState(t, rs.ledger).Busy())

	require.NoError(t, rs.StopChat(t.Context(), "chat-1"))

	assert.False(t, chatState(t, rs.ledger).Busy())
}

func TestSettleStaleWork_LetsAResumeThroughAChatWhoseRunnerIsGone(t *testing.T) {
	rs := retiring(t, &Runners{chatLock: newTestLock(), parks: newParks(), turns: noopTurns{}})
	rs.live = liveWith(t)
	runnerStarted(t, rs, true)
	require.ErrorIs(t, rs.requireNotBusy(t.Context(), "chat-1"), ErrPromptBusy)

	require.NoError(t, rs.settleStaleWork(t.Context(), "chat-1"))

	require.NoError(t, rs.requireNotBusy(t.Context(), "chat-1"))
}

func TestSettleStaleWork_LeavesALiveRunnersTurnRunning(t *testing.T) {
	rs := retiring(t, &Runners{chatLock: newTestLock(), parks: newParks(), turns: noopTurns{}})
	runnerStarted(t, rs, true)

	require.NoError(t, rs.settleStaleWork(t.Context(), "chat-1"))

	assert.True(t, chatState(t, rs.ledger).Busy())
}
