package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
)

// CloseStalledTurn exposes the stall closer to this package's (external) tests.
// It is not production surface: this file is compiled only under `go test`.
//
// The closer is reached directly rather than through a sweep because the two are
// separate contracts with separate tests. The sweep owns WHETHER a turn has been
// abandoned — six gates, a clock, and a screen — and is tested exhaustively in the
// termwait package against fakes. This owns WHAT HAPPENS when it has been, which
// is a turn command and a ledger write against the real event stores, and driving
// it through a detector would mean simulating a wedged PTY to test a database
// write.
func CloseStalledTurn(u *Usecase, ctx context.Context, stall termwait.Stall) {
	u.closeStalledTurn(ctx, stall)
}
