package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
)

func CloseStalledTurn(u *Usecase, ctx context.Context, stall termwait.Stall) {
	u.closeStalledTurn(ctx, stall)
}
