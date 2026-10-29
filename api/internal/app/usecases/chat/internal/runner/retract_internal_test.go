package runner

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// signalTerm is a PTY that takes its quit signal and exits later, noting the
// chat's last ledger row at the moment it was signalled.
type signalTerm struct {
	fakeTermForAttach
	ledger *ledgerstore.Store
	mu     sync.Mutex
	at     []ledger.Row
}

func (s *signalTerm) TerminateGraceful(ctx context.Context, sessionID string) error {
	rows, err := s.ledger.Read(ctx, "chat-1", 0, 0)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminated = append(s.terminated, sessionID)
	s.at = append(s.at, rows[len(rows)-1])
	return nil
}

func (s *signalTerm) signalled() []ledger.Row {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ledger.Row(nil), s.at...)
}

// retracting is runner-1 live on chat-1 behind PTY pty-1, its quit signal
// noted by the returned term.
func retracting(t *testing.T) (*Runners, *signalTerm) {
	t.Helper()
	l := newTestLedger(t)
	term := &signalTerm{ledger: l}
	rs := retiring(t, &Runners{
		apiConns: newAPIConnRegistry(),
		attached: newAttachRegistry(),
		ws:       stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:   stubAgentsForInterrupt{agent: interruptTestAgent(t)},
		ledger:   l,
		chatLock: newTestLock(), parks: newParks(),
		turns: noopTurns{},
		term:  term,
	})
	pty := interruptRunner
	pty.TerminalSession = "pty-1"
	rs.live = liveWith(t, pty)
	runnerStarted(t, rs, false)
	return rs, term
}

func appendRow[B any](t *testing.T, rs *Runners, kind ledger.Kind, turnID string, body B) {
	t.Helper()
	row, err := ledger.NewRow(kind, turnID, "", body)
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
}

// sent records a crowbar prompt r1 carried by runner-1 under delivery.
func sent(t *testing.T, rs *Runners, delivery string) {
	t.Helper()
	appendRow(t, rs, ledger.KindUserMessage, "U-1", ledger.UserMessage{
		RequestID: "r1", Text: "rename it", Origin: ledger.OriginCrowbar, Delivery: delivery, RunnerID: "runner-1",
	})
}

// took records runner-1 taking r1 as its running turn.
func took(t *testing.T, rs *Runners) {
	t.Helper()
	appendRow(t, rs, ledger.KindTurnStarted, "T-1", ledger.TurnStarted{RequestID: "r1", RunnerID: "runner-1"})
}

func TestRetract_APendingPromptSettlesRetractedAndReturnsItsText(t *testing.T) {
	rs, term := retracting(t)
	sent(t, rs, ledger.DeliveryAPI)

	got, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.NoError(t, err)
	assert.Equal(t, RetractResult{State: RetractRetracted, Text: "rename it"}, got)
	p, ok := chatState(t, rs.ledger).Receipt("r1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, p.Status)
	assert.False(t, p.Consumed)
	assert.Equal(t, ledger.ReasonRetracted, p.Reason)
	assert.Empty(t, chatState(t, rs.ledger).PendingPrompts(), "the chat takes the next prompt at once")
	assert.Empty(t, term.signalled(), "a prompt pushed over a live connection leaves its CLI alone")
}

func TestRetract_ARestartDeliveredPromptTerminatesTheRunnerItStarted(t *testing.T) {
	rs, term := retracting(t)
	sent(t, rs, ledger.DeliveryRestart)

	got, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.NoError(t, err)
	assert.Equal(t, RetractRetracted, got.State)
	signalled := term.signalled()
	require.Len(t, signalled, 1, "the CLI launched to carry r1 is told to quit")
	assert.Equal(t, ledger.KindPromptSettled, signalled[0].Kind, "the prompt settles before its CLI is signalled")
	assert.Empty(t, retiredAs(t, rs), "Retract does not wait for the CLI's exit")

	rs.reconcileRunnerExit(t.Context(), "runner-1") // the PTY's exit callback
	assert.Equal(t, domain.AgentExitRetracted, retiredAs(t, rs))
}

func TestRetract_ASteerDeliveredPromptLeavesTheRunnerRunning(t *testing.T) {
	rs, term := retracting(t)
	appendRow(t, rs, ledger.KindTurnStarted, "T-0", ledger.TurnStarted{RunnerID: "runner-1"})
	sent(t, rs, ledger.DeliverySteer)

	got, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.NoError(t, err)
	assert.Equal(t, RetractResult{State: RetractRetracted, Text: "rename it"}, got)
	assert.Empty(t, term.signalled(), "the CLI the prompt was steered into keeps running its turn")
	turn, running := chatState(t, rs.ledger).RunningTurn()
	assert.True(t, running)
	assert.Equal(t, "T-0", turn.ID, "the turn r1 was steered into is not the retract's to end")
	v, _ := rs.live.view("chat-1")
	assert.False(t, v.exiting)
}

func TestRetract_ARunningTurnEndsInterruptedBeforeTheProviderIsSignalled(t *testing.T) {
	rs, term := retracting(t)
	sent(t, rs, ledger.DeliveryRestart)
	took(t, rs)

	got, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.NoError(t, err)
	assert.Equal(t, RetractResult{State: RetractInterrupted, Text: "rename it"}, got)
	signalled := term.signalled()
	require.Len(t, signalled, 1, "a provider that cannot interrupt in place is retired, as Stop does")
	require.Equal(t, ledger.KindTurnEnded, signalled[0].Kind)
	ended, err := ledger.DecodeBody[ledger.TurnEnded](signalled[0])
	require.NoError(t, err)
	assert.Equal(t, ledger.TurnEnded{
		Status: string(ledger.TurnStatusInterrupted), Reason: ledger.ReasonRetracted, RunnerID: "runner-1",
	}, ended)
	assert.False(t, chatState(t, rs.ledger).Busy())
}

func TestRetract_ASettledPromptIsAConflict(t *testing.T) {
	t.Run("settled", func(t *testing.T) {
		rs, term := retracting(t)
		sent(t, rs, ledger.DeliveryRestart)
		appendRow(t, rs, ledger.KindPromptSettled, "", ledger.PromptSettled{RequestID: "r1", Reason: ledger.ReasonRunnerExited})

		_, err := rs.Retract(t.Context(), "chat-1", "r1")

		require.ErrorIs(t, err, problem.ErrConflict)
		assert.Empty(t, term.signalled())
	})
	t.Run("its turn closed", func(t *testing.T) {
		rs, term := retracting(t)
		sent(t, rs, ledger.DeliveryRestart)
		took(t, rs)
		appendRow(t, rs, ledger.KindTurnEnded, "T-1", ledger.TurnEnded{Status: string(ledger.TurnStatusDone)})
		head := chatState(t, rs.ledger).Head()

		_, err := rs.Retract(t.Context(), "chat-1", "r1")

		require.ErrorIs(t, err, problem.ErrConflict)
		assert.Equal(t, head, chatState(t, rs.ledger).Head(), "a refused retract records nothing")
		assert.Empty(t, term.signalled())
	})
}

func TestRetract_AnUnknownRequestIsNotFound(t *testing.T) {
	rs, term := retracting(t)
	sent(t, rs, ledger.DeliveryRestart)

	_, err := rs.Retract(t.Context(), "chat-1", "r2")

	require.ErrorIs(t, err, problem.ErrNotFound)
	assert.Equal(t, []ledger.Prompt{promptReceipt(t, rs.ledger, "r1")}, chatState(t, rs.ledger).PendingPrompts())
	assert.Empty(t, term.signalled())
}

// gapLock runs gap once, right after the lock's next holder lets go: the first
// path to take the chat after that holder.
type gapLock struct {
	ChatLock
	gap func()
}

func (g *gapLock) Lock(ctx context.Context, chatID string) (func(), error) {
	unlock, err := g.ChatLock.Lock(ctx, chatID)
	gap := g.gap
	g.gap = nil
	if err != nil || gap == nil {
		return unlock, err
	}
	return func() { unlock(); gap() }, nil
}

// A send that takes the chat the moment Retract lets go must find the
// retracted prompt's CLI already ending, never deliver onto it.
func TestRetract_ASendRightAfterARetractNeverLandsOnTheRetiringCLI(t *testing.T) {
	rs, _ := retracting(t)
	sent(t, rs, ledger.DeliveryRestart)
	lock := &gapLock{ChatLock: rs.chatLock}
	rs.chatLock = lock
	lock.gap = func() { // what a send decides from: settledRunner's view
		unlock, err := lock.Lock(t.Context(), "chat-1")
		require.NoError(t, err)
		defer unlock()
		if v, _ := rs.live.view("chat-1"); !v.exiting {
			appendRow(t, rs, ledger.KindUserMessage, "U-2", ledger.UserMessage{
				RequestID: "r2", Text: "two", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryAPI, RunnerID: "runner-1",
			})
		}
	}

	_, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.NoError(t, err)
	_, sentTwo := chatState(t, rs.ledger).Receipt("r2")
	assert.False(t, sentTwo, "the next send waits out the CLI the retract retires")
}

func TestRetract_ADeliveredPromptWhoseTurnClosedNeverEndsALaterTurn(t *testing.T) {
	rs, term := retracting(t)
	sent(t, rs, ledger.DeliveryRestart)
	took(t, rs)
	appendRow(t, rs, ledger.KindTurnEnded, "T-1", ledger.TurnEnded{Status: string(ledger.TurnStatusDone)})
	appendRow(t, rs, ledger.KindTurnStarted, "T-2", ledger.TurnStarted{RunnerID: "runner-1"})

	_, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.ErrorIs(t, err, problem.ErrConflict)
	turn, running := chatState(t, rs.ledger).RunningTurn()
	assert.True(t, running)
	assert.Equal(t, "T-2", turn.ID, "the turn running now is not r1's")
	assert.Empty(t, term.signalled())
}

// Mid-displace the chat's live CLI is the outgoing one, not r1's carrier.
func TestRetract_APromptWhoseCarrierIsNotLiveYetLeavesTheOutgoingCLIAlone(t *testing.T) {
	rs, term := retracting(t)
	appendRow(t, rs, ledger.KindUserMessage, "U-1", ledger.UserMessage{
		RequestID: "r1", Text: "rename it", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart, RunnerID: "runner-2",
	})

	got, err := rs.Retract(t.Context(), "chat-1", "r1")

	require.NoError(t, err)
	assert.Equal(t, RetractRetracted, got.State)
	assert.Empty(t, term.signalled(), "the outgoing CLI's end is its displacer's")
	v, _ := rs.live.view("chat-1")
	assert.False(t, v.exiting)
}

func TestRetract_AnInterruptibleProviderKeepsItsCLIOnlyForADeliveredPrompt(t *testing.T) {
	t.Run("delivered: interrupted in place", func(t *testing.T) {
		rs, term := retracting(t)
		asked, release := make(chan struct{}), make(chan struct{})
		defer close(release)
		rs.apiConns.set("runner-1", interruptServer(t, asked, release))
		sent(t, rs, ledger.DeliveryAPI)
		took(t, rs)

		got, err := rs.Retract(t.Context(), "chat-1", "r1")

		require.NoError(t, err)
		assert.Equal(t, RetractInterrupted, got.State)
		require.Empty(t, term.signalled(), "an interruptible provider keeps its CLI")
		<-asked
	})
	t.Run("pending: the carrier is retired", func(t *testing.T) {
		rs, term := retracting(t)
		rs.apiConns.set("runner-1", interruptServer(t, make(chan struct{}), nil))
		sent(t, rs, ledger.DeliveryRestart)

		_, err := rs.Retract(t.Context(), "chat-1", "r1")

		require.NoError(t, err)
		assert.Len(t, term.signalled(), 1, "no turn runs to interrupt: the CLI carrying r1 goes")
	})
}

// subagentOn records subagent scope live under turnID.
func subagentOn(t *testing.T, rs *Runners, turnID, scope string) {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, scope, ledger.SubagentBody{Phase: ledger.SubagentStart, Type: "explorer"})
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
}

func TestRetract_InterruptsTheRetractedTurnsSubagentsOnly(t *testing.T) {
	t.Run("its own: interrupted, the chat takes the next prompt", func(t *testing.T) {
		rs, _ := retracting(t)
		sent(t, rs, ledger.DeliveryRestart)
		took(t, rs)
		subagentOn(t, rs, "T-1", "A")

		_, err := rs.Retract(t.Context(), "chat-1", "r1")

		require.NoError(t, err)
		st := chatState(t, rs.ledger)
		a, _ := st.SubagentOf("A")
		assert.Equal(t, ledger.SubagentInterrupted, a.Status)
		assert.False(t, st.Busy())
	})
	t.Run("an earlier turn's: still running, the chat still busy", func(t *testing.T) {
		rs, _ := retracting(t)
		appendRow(t, rs, ledger.KindTurnStarted, "T-0", ledger.TurnStarted{RunnerID: "runner-1"})
		subagentOn(t, rs, "T-0", "B")
		appendRow(t, rs, ledger.KindTurnEnded, "T-0", ledger.TurnEnded{Status: string(ledger.TurnStatusDone)})
		sent(t, rs, ledger.DeliveryRestart)
		took(t, rs)

		_, err := rs.Retract(t.Context(), "chat-1", "r1")

		require.NoError(t, err)
		st := chatState(t, rs.ledger)
		b, _ := st.SubagentOf("B")
		assert.True(t, b.Status.Live(), "a background subagent of an earlier turn is not r1's to stop")
		assert.True(t, st.Busy())
	})
}
