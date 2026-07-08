package reviewthread

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// MaxOCCAttempts exposes the OCC retry bound so external tests can assert the
// ErrPipelineFailed disposition contract (retry ≤5×; spec §3.5, decision 10).
const MaxOCCAttempts = maxOCCAttempts

// OccSend exposes the OCC retry + terminal error-disposition helper so external
// tests can drive it against a fake send closure (forcing ErrPipelineFailed /
// ErrValidation / ErrQueueFull) without standing up a real asynx.
func OccSend(
	ctx context.Context,
	send func(context.Context, asynxModels.Command[domain.ReviewThread]) (asynxModels.Event[domain.ReviewThread], error),
	cmd asynxModels.Command[domain.ReviewThread],
) (asynxModels.Event[domain.ReviewThread], error) {
	return occSend(ctx, send, cmd)
}
