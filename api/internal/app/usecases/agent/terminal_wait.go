package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (u *runnerUsecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	if u.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return u.termWait.Wait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep and wires the two publish
// callbacks the hub owns: promptSettled here, messageDelta onto the turn
// concern.
//
// Both are assigned BEFORE the nil-detector return. A daemon with no detector
// still streams assistant messages to its chat UI, and dropping messageDelta on
// that path is invisible until a user watches a message that never grows.
func (u *runnerUsecase) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
	messageDelta func(chatID, workspaceID, messageID, text string),
) {
	u.promptSettled = promptSettled
	u.turn.messageDelta = messageDelta
	if u.termWait == nil {
		return
	}
	u.termWait.Run(ctx, publish)
}

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
// whole of the "no detector" case every reader of termWait guards for.
func newTerminalWaitDetector(
	chat *chatUsecase,
	turn *turnUsecase,
	runner *runnerUsecase,
) termwait.Detector {
	screens, ok := runner.term.(termwait.Screens)
	if !ok {
		return nil
	}
	return termwait.New(termwait.Deps{
		Runners: runner.runners,
		Chats:   chat.chats,
		Choices: turn.activity,
		Screens: screens,
		Prompts: turn,

		Notices: turn,
		Work:    turn,
		OnStall: turn.closeStalledTurn,

		Deliveries: runner,

		Messages: turn,
	})
}
