package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (u *Usecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	if u.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return u.termWait.Wait(chatID)
}

func (u *Usecase) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
	messageDelta func(chatID, workspaceID, messageID, text string),
) {
	u.promptSettled = promptSettled
	u.messageDelta = messageDelta
	if u.termWait == nil {
		return
	}
	u.termWait.Run(ctx, publish)
}

func (u *Usecase) MatchTerminalPrompt(
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

func newTerminalWaitDetector(u *Usecase) termwait.Detector {
	screens, ok := u.term.(termwait.Screens)
	if !ok {

		return nil
	}
	return termwait.New(termwait.Deps{
		Runners: u.runners,
		Chats:   u.chats,
		Choices: u.activity,
		Screens: screens,
		Prompts: u,

		Notices: u,
		Work:    u,
		OnStall: u.closeStalledTurn,

		Deliveries: u,

		Messages: u,
	})
}
