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

var _ termwait.Screens = (engineterminal.Engine)(nil)

func TestUsecase_TerminalWait_WithoutADetectorIsNotWaiting(t *testing.T) {
	u := &Usecase{}

	assert.False(t, u.TerminalWait("any-chat").Waiting)

	u.StartTerminalWaitSweep(t.Context(), nil, nil, nil)
}

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

func TestUsecase_TerminalWait_ReadsThroughTheDetector(t *testing.T) {
	u := &Usecase{term: screenReadingCommander{}}
	u.termWait = newTerminalWaitDetector(u)

	require.NotNil(t, u.termWait)
	assert.False(t, u.TerminalWait("chat-1").Waiting)
}

func TestUsecase_MatchTerminalPrompt_UnresolvableHomeIsSilent(t *testing.T) {
	u := &Usecase{
		agents: engineagents.New(),
		home:   func() (string, error) { return "", errors.New("no home") },
	}

	_, ok := u.MatchTerminalPrompt(t.Context(), "claude", "❯ 1. Yes, I trust this folder")

	assert.False(t, ok)
}

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

type sweepRecorder struct{ swept chan struct{} }

func (sweepRecorder) Wait(string) domain.AgentTerminalWait { return domain.AgentTerminalWait{} }

func (sweepRecorder) Sweep(context.Context, termwait.Publish) {}

func (r sweepRecorder) Run(context.Context, termwait.Publish) { r.swept <- struct{}{} }
