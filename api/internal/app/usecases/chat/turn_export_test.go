package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
)

// WaitingForTurnLog is the log record a provider switch emits at the INSTANT it parks on
// an in-flight turn, exposed to this package's (external) tests. It is not production
// surface: this file is compiled only under `go test`.
//
// It exists so a test can block on THE SWITCH BEING PARKED — a real, causal signal —
// instead of sleeping and hoping. That matters more here than anywhere else in the
// package: the property under test is a NEGATIVE ("the outgoing CLI is not killed while
// the turn is still running"), and a negative can only be proven against a moment the
// test knows the switch has actually reached.
const WaitingForTurnLog = turn.WaitingForTurnLog

// HookDeliveryCount reports how many completed delivery ids the ingress's
// in-memory dedup set holds.
func HookDeliveryCount(u TurnUsecase) int {
	uc, ok := u.(*Usecase)
	if !ok {
		panic(fmt.Sprintf("chat: HookDeliveryCount given %T, not the chat usecase", u))
	}
	return uc.turns.HookDeliveryCount()
}

func CloseStalledTurn(u TurnUsecase, ctx context.Context, stall seam.Stall) {
	u.(*Usecase).turns.CloseStalledTurn(ctx, stall)
}

// SetMessageAwaitTimeout overrides how long closeAssistantTurn waits for a
// still-streaming message before concluding nothing streamed (see
// stream.Streams.AwaitOpen). Test-only surface: production always uses
// turn.defaultMessageAwaitTimeout.
func SetMessageAwaitTimeout(u TurnUsecase, d time.Duration) {
	u.(*Usecase).turns.SetMessageAwaitTimeout(d)
}
