package termwait_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// A notice that ends the turn fails it the moment it appears on a busy chat's
// screen: the screen is the only place the provider says so.
func TestDetector_Sweep_ReportsANoticeThatEndsTheTurn(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()

	got := r.ended.all()
	require.Len(t, got, 1)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, got[0].Kind)
	assert.Equal(t, "runner-1", r.ended.on[0].ID)
	assert.Empty(t, r.rec.all(), "a busy chat is never waiting")
}

func TestDetector_Sweep_ANoticeIsReportedOnceWhileItStays(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()
	r.screens.set(session, usageLimitScreen+"\n  more output")
	r.sweep()
	r.screens.repaint(session)
	r.sweep()

	assert.Len(t, r.ended.all(), 1)
}

func TestDetector_Sweep_ANoticeOnAReplacedRunnersScreenIsReportedAgain(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.runners.live[0].TerminalSession = "pty-2"
	r.screens.set("pty-2", usageLimitScreen)
	r.sweep()

	assert.Len(t, r.ended.all(), 2)
}

func TestDetector_Sweep_ANoticeReturningAfterItScrolledAwayIsReportedAgain(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.screens.set(session, idleScreen)
	r.sweep()
	r.screens.set(session, usageLimitScreen)
	r.sweep()

	assert.Len(t, r.ended.all(), 2)
}

func TestDetector_Sweep_ANoticeNeverEndsAnIdleChatsTurn(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.busy.set(false)

	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_ABannerLeftFromAnEarlierTurnEndsNothing(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.busy.set(false)
	r.sweep()

	r.busy.set(true)
	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_NoticeWithoutEndsTurnEndsNothing(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.notices.needles["codex"] = []engineagents.TerminalNotice{{
		Kind:   engineagents.TerminalNoticeUsageLimit,
		Needle: usageLimitNeedle,
		Text:   usageLimitSentence,
	}}

	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_ProviderDeclaringNoNoticesEndsNothing(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.runners.live[0].ProviderID = "claude"

	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_WithoutNoticeDependenciesNothingEnds(t *testing.T) {
	r := newRig(t)
	r.wedged()
	waitOnly := termwait.New(termwait.Deps{
		Runners: r.runners,
		Work:    r.busy,
		Choices: r.choices,
		Screens: r.screens,
		Prompts: r.prompts,
	})

	waitOnly.Sweep(context.Background(), r.rec.publish)

	assert.Empty(t, r.ended.all())
	assert.Zero(t, r.notices.asked)
}

// A choice the person has yet to make holds the turn: a notice that would end
// it waits until the choice is made.
func TestDetector_Sweep_ANoticeWaitsForAPendingChoice(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.choices.pending[chatID] = []domain.ActivityChoice{{ID: "choice-1", ChatID: chatID}}

	r.sweep()
	assert.Empty(t, r.ended.all(), "a turn holding a choice is not ended by its screen")

	delete(r.choices.pending, chatID)
	r.sweep()
	assert.Len(t, r.ended.all(), 1, "once the choice is made, the notice ends the turn")
}

// A prompt awaiting its provider, with no turn running, is ended by a notice
// too: the provider will never take it.
func TestDetector_Sweep_ANoticeEndsAnAwaitedPrompt(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.busy.set(false)
	r.busy.await(true)

	r.sweep()

	require.Len(t, r.ended.all(), 1)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, r.ended.all()[0].Kind)
}
