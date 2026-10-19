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

// HookDeliveryCount reports how many completed delivery ids the ingress's
// in-memory dedup set holds.
func HookDeliveryCount(u TurnUsecase) int {
	return usecaseOf(u).turns.HookDeliveryCount()
}

// FailTurnOnNotice runs what the terminal-wait sweep does once a runner's
// screen shows a notice that ends its turn.
func FailTurnOnNotice(
	ctx context.Context, u TurnUsecase, runner engineagents.Runner, notice engineagents.TerminalNotice,
) {
	usecaseOf(u).turns.FailTurnOnNotice(ctx, runner, notice)
}
