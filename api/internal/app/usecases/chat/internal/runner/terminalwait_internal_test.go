package runner

import (
	"context"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
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

	rs.StartTerminalWaitSweep(t.Context(), nil, nil, nil, nil)

	<-swept
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

type blindTerminal struct{}

func (blindTerminal) CreateCommand(
	context.Context, string, string, []string, []string, func(),
) (string, error) {
	return "", nil
}

func (blindTerminal) TerminateGraceful(context.Context, string) error { return nil }

func (blindTerminal) SessionLive(context.Context, string) bool { return false }

type seeingTerminal struct{ blindTerminal }

func (seeingTerminal) Screen(string, uint64) (string, uint64, bool) { return "", 0, false }

// noTurns is the hook-ingress port with nothing behind it. SetTurns only has to
// be able to bind it.
type noTurns struct{ Turns }

func (noTurns) SetMessageDelta(func(chatID, workspaceID, messageID, text string)) {}

func (noTurns) SetCompactionStatus(func(chatID, workspaceID string, active bool)) {}

func (noTurns) CloseStalledTurn(context.Context, seam.Stall) {}
