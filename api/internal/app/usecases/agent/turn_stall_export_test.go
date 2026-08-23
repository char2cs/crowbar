package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
)

func CloseStalledTurn(u TurnUsecase, ctx context.Context, stall termwait.Stall) {
	u.(*turnUsecase).closeStalledTurn(ctx, stall)
}
