package agentchat

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// MaxOCCAttempts exposes the OCC retry bound so external tests can assert the
// ErrPipelineFailed disposition contract.
const MaxOCCAttempts = maxOCCAttempts

// WaitQuiescentForTest blocks until the EventStore's asynx instance has
// drained its dispatch queue and run every projection handler (WaitPublish).
// sendWithOCC dispatches via ax.Send (not SendWait), so the store/hub
// projections still run asynchronously after a mutation returns; an external
// test calls this after a mutation to read the store projection with no
// polling and no timeouts.
func WaitQuiescentForTest(
	es EventStore,
) {
	es.(*eventSourced).ax.WaitPublish()
}

// OccSend exposes the OCC retry + terminal error-disposition helper so
// external tests can drive it against a fake send closure (forcing
// ErrPipelineFailed / ErrValidation / ErrQueueFull) without standing up a
// real asynx.
func OccSend(
	ctx context.Context,
	send func(context.Context, asynxModels.Command[domain.AgentChat]) (asynxModels.Event[domain.AgentChat], error),
	cmd asynxModels.Command[domain.AgentChat],
) (asynxModels.Event[domain.AgentChat], error) {
	return occSend(ctx, send, cmd)
}
