package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// The real terminal engine MUST satisfy the detector's screen seam.
//
// This is a compile-time assertion because the wiring that connects them is a
// GUARDED TYPE ASSERTION (newTerminalWaitDetector), chosen so a terminal double
// that cannot render a screen simply has no detector rather than having to
// implement one. The cost of that choice is that a changed Engine.Screen
// signature would not break the build — it would silently stop the detector ever
// being constructed, and "the agent is waiting in the terminal" would quietly
// never be reported again, in production only.
//
// So the guarantee the assertion gives up is pinned here instead.
var _ termwait.Screens = (engineterminal.Engine)(nil)

// TestUsecase_TerminalWait_WithoutADetectorIsNotWaiting is the degradation
// contract stated where it is relied on: a usecase whose terminal seam cannot
// render a screen answers the zero verdict for every chat, which is exactly what
// every chat answered before this existed.
func TestUsecase_TerminalWait_WithoutADetectorIsNotWaiting(t *testing.T) {
	u := &Usecase{}

	assert.False(t, u.TerminalWait("any-chat").Waiting)
	// And starting the sweep on one is a no-op rather than a nil dereference: the
	// app wires this unconditionally, and a daemon must not fail to boot because a
	// capability it does not have was switched on.
	u.StartTerminalWaitSweep(t.Context(), nil, nil, nil)
}

// screenReadingCommander is a terminal seam that CAN render a screen — the shape
// the production engine has, and the one the detector's construction requires.
type screenReadingCommander struct{}

func (screenReadingCommander) CreateCommand(
	context.Context, string, string, []string, []string, func(),
) (string, error) {
	return "", nil
}

func (screenReadingCommander) TerminateGraceful(context.Context, string) error { return nil }

func (screenReadingCommander) SessionLive(context.Context, string) bool { return false }

func (screenReadingCommander) Screen(string, uint64) (string, uint64, bool) {
	return "", 0, false
}

// TestUsecase_TerminalWait_ReadsThroughTheDetector is the other side of the
// degradation contract: a terminal seam that CAN render a screen gets a detector,
// and the read goes through it. Nothing has swept yet, so the honest answer is
// still "not waiting" — which is what a chat reports for the moment between the
// daemon starting and its first sweep.
func TestUsecase_TerminalWait_ReadsThroughTheDetector(t *testing.T) {
	u := &Usecase{term: screenReadingCommander{}}
	u.termWait = newTerminalWaitDetector(u)

	require.NotNil(t, u.termWait)
	assert.False(t, u.TerminalWait("chat-1").Waiting)
}

// TestUsecase_MatchTerminalPrompt_UnresolvableHomeIsSilent: home resolution can
// fail, and the answer to that is silence rather than an error. A sweep over every
// live runner must not be able to fail on one of them, and a provider whose
// descriptor cannot be resolved declares no needles — which is exactly the same
// answer as a provider that declares none.
func TestUsecase_MatchTerminalPrompt_UnresolvableHomeIsSilent(t *testing.T) {
	u := &Usecase{
		agents: engineagents.New(),
		home:   func() (string, error) { return "", errors.New("no home") },
	}

	_, ok := u.MatchTerminalPrompt(t.Context(), "claude", "❯ 1. Yes, I trust this folder")

	assert.False(t, ok)
}

// TestUsecase_StartTerminalWaitSweep_DrivesTheDetector proves the sweep is really
// started rather than merely not crashing: the loop publishes nothing here (the
// census is empty), so the observable effect is that the detector's own Run was
// reached, which the census read records.
func TestUsecase_StartTerminalWaitSweep_DrivesTheDetector(t *testing.T) {
	swept := make(chan struct{}, 1)
	u := &Usecase{}
	u.termWait = sweepRecorder{swept: swept}

	u.StartTerminalWaitSweep(t.Context(), nil, nil, nil)

	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("StartTerminalWaitSweep did not drive the detector")
	}
}

// sweepRecorder is the detector reduced to "was I started?".
type sweepRecorder struct{ swept chan struct{} }

func (sweepRecorder) Wait(string) domain.AgentTerminalWait { return domain.AgentTerminalWait{} }

func (sweepRecorder) Sweep(context.Context, termwait.Publish) {}

func (r sweepRecorder) Run(context.Context, termwait.Publish) { r.swept <- struct{}{} }
