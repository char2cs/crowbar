package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (u *turnUsecase) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	home, err := u.home()
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	descriptor, err := u.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	return descriptor.MatchTerminalPrompt(screen)
}

// newTerminalWaitDetector builds the detector LAST, after the concern types
// exist: its ports are spread across them (the prompt and notice matchers, the
// open-work read, the message stream and the prompt-delivery journal), and it
// binds them by value. Building it earlier would bind nil.
//
// It returns NIL when the terminal seam cannot render a screen, which is the
// whole of the "no detector" case every reader of u.termWait guards for.
func newTerminalWaitDetector(u *Usecase) termwait.Detector {
	screens, ok := u.runner.term.(termwait.Screens)
	if !ok {
		return nil
	}
	return termwait.New(termwait.Deps{
		Runners: u.runner.runners,
		Chats:   u.chat.chats,
		Choices: u.turn.activity,
		Screens: screens,
		Prompts: u.turn,

		Notices: u.turn,
		Work:    u.turn,
		OnStall: u.turn.closeStalledTurn,

		Deliveries: u.runner,

		Messages: u.turn,
	})
}
