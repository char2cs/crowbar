package chat

import (
	"context"
	"fmt"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func usecaseOf(u TurnUsecase) *Usecase {
	uc, ok := u.(*Usecase)
	if !ok {
		panic(fmt.Sprintf("chat: given %T, not the chat usecase", u))
	}
	return uc
}

func FailTurnOnNotice(
	ctx context.Context, u TurnUsecase, runner engineagents.Runner, notice engineagents.TerminalNotice,
) {
	usecaseOf(u).recorder.FailTurnOnNotice(ctx, runner, notice)
}
