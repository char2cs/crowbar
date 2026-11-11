package operation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// logFailure keeps the whole chain the operation's reason leaves out.
func logFailure(ctx context.Context, op domain.Operation, err error) {
	if err != nil {
		slog.ErrorContext(ctx, "operation failed", "op", op.ID, "kind", op.Kind, "err", err)
	}
}

func settle(
	ctx context.Context,
	op *domain.Operation,
	err error,
) {
	op.State = domain.OpFailed
	switch failed := failedItems(op.Items); {
	case err != nil && ctx.Err() != nil:
		op.Error = &problem.Problem{Code: problem.Interrupted, Message: interruptedMessage}
	case err != nil:
		p := problem.Of(err)
		op.Error = &p
	case failed > 0:
		op.Error = &problem.Problem{Code: problem.Partial, Message: fmt.Sprintf("%d of %d failed", failed, len(op.Items))}
	default:
		op.State = domain.OpSucceeded
	}
	settleItems(op)
}

func failedItems(items []domain.OperationItem) int {
	n := 0
	for _, item := range items {
		if item.Error != nil || item.State == domain.OpFailed {
			n++
		}
	}
	return n
}

// settleItems ends with op the items its work left open: no item runs on once
// its operation has settled. Work that returned finished them; work that
// failed or was cut short failed them with its own reason.
func settleItems(op *domain.Operation) {
	if !settled(op.State) {
		return
	}
	finished := op.State == domain.OpSucceeded || op.Error != nil && op.Error.Code == problem.Partial
	for i := range op.Items {
		item := &op.Items[i]
		switch {
		case settled(item.State):
		case item.Error != nil:
			item.State = domain.OpFailed
		case finished:
			item.State = domain.OpSucceeded
		default:
			reason := *op.Error
			item.State, item.Error = domain.OpFailed, &reason
		}
	}
}
