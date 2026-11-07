package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TestStartTerminalWaitSweep_DrivesTheDetector is white-box because the thing
// under test is the one call nothing outside can observe: a sweep that wired its
// callbacks and then never started would look identical from the API, and every
// chat would silently stop reporting a CLI parked on a modal.
//
// The detector is substituted directly rather than through a constructor door: a
// production path that could take a detector from a caller would be a path where
// SetTurns is not the single place one is built.
func TestStartTerminalWaitSweep_DrivesTheDetector(t *testing.T) {
	t.Parallel()

	swept := make(chan struct{}, 1)
	rs := New(Deps{})
	rs.turns = noTurns{}
	rs.termWait = sweepRecorder{swept: swept}

	rs.StartTerminalWaitSweep(t.Context())

	<-swept
}

// A dialog a command opens is that command running in the CLI: the prompt is
// taken, so the chat waits on the dialog, never on the prompt.
func TestStartTerminalWaitSweep_ACommandsDialogIsTheCommandRunning(t *testing.T) {
	claude, err := agents.New().Get(t.Context(), "", "claude")
	require.NoError(t, err)
	rs := &Runners{
		ledger: newTestLedger(t),
		ws:     stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents: stubAgentsForInterrupt{agent: claude},
		live:   liveWith(t, agents.Runner{ID: "runner-1", ProviderID: "claude", CurrentChatID: "chat-1"}),
	}
	appendRow(t, rs, ledger.KindUserMessage, "U-1", ledger.UserMessage{
		RequestID: "r1", Text: "/help", Origin: ledger.OriginCrowbar,
		Delivery: ledger.DeliveryRestart, RunnerID: "runner-1",
	})
	published := make(chan struct{})
	rs.termWait = waitPublisher{chatID: "chat-1", wait: domain.AgentTerminalWait{Waiting: true}, done: published}

	rs.StartTerminalWaitSweep(t.Context())
	<-published

	st := chatState(t, rs.ledger)
	assert.Empty(t, st.PendingPrompts())
	p, ok := st.Receipt("r1")
	require.True(t, ok)
	assert.True(t, p.Consumed)
	assert.Equal(t, ledger.ReasonProviderBuiltin, p.Reason)
}

type waitPublisher struct {
	sweepRecorder
	chatID string
	wait   domain.AgentTerminalWait
	done   chan struct{}
}

func (w waitPublisher) Run(ctx context.Context, publish termwait.Publish) {
	publish(w.chatID, "", w.wait)
	close(w.done)
}

// TestSetTurns_BuildsADetectorOnlyForAScreenReadingTerminal pins the branch every
// reader of termWait guards for.
func TestSetTurns_BuildsADetectorOnlyForAScreenReadingTerminal(t *testing.T) {
	t.Parallel()

	blind := New(Deps{Terminal: blindTerminal{}})
	blind.SetTurns(noTurns{})
	if blind.termWait != nil {
		t.Fatal("a terminal that cannot render a screen must leave the daemon with no detector")
	}

	seeing := New(Deps{Terminal: seeingTerminal{}})
	seeing.SetTurns(noTurns{})
	if seeing.termWait == nil {
		t.Fatal("a screen-reading terminal must get a detector; without one no chat ever " +
			"reports a CLI parked on a modal")
	}
}

type sweepRecorder struct{ swept chan struct{} }

func (sweepRecorder) Wait(string) domain.AgentTerminalWait { return domain.AgentTerminalWait{} }

func (sweepRecorder) Sweep(context.Context, termwait.Publish) {}

func (r sweepRecorder) Run(context.Context, termwait.Publish) { r.swept <- struct{}{} }

func (sweepRecorder) Wake() {}

type blindTerminal struct{}

func (blindTerminal) CreateCommand(
	context.Context, string, string, []string, []string, func(engineterminal.Exit),
) (string, error) {
	return "", nil
}

func (blindTerminal) TerminateGraceful(context.Context, string) error { return nil }

func (blindTerminal) Write(context.Context, string, []byte) error { return nil }

func (blindTerminal) SessionLive(context.Context, string) bool { return false }

type seeingTerminal struct{ blindTerminal }

func (seeingTerminal) Screen(string, uint64) (string, uint64, bool) { return "", 0, false }

// noTurns is the hook-ingress port with nothing behind it. SetTurns only has to
// be able to bind it.
type noTurns struct{ Turns }

func (noTurns) FailTurnOnNotice(context.Context, agents.Runner, agents.TerminalNotice) {}
