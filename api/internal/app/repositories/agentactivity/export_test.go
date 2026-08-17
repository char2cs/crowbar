package agentactivity

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DispatchForTest exposes the OCC retry and error-classification policy.
//
// It is exported to the package's own tests because the policy is a decision, not
// an implementation detail: which failures are retried and which are terminal is
// the difference between a wedged prompt and a duplicated one, and driving it
// through a real sharded asynx would test the scheduler instead.
func DispatchForTest(
	ctx context.Context,
	send func(context.Context, asynxModels.Command[domain.AgentActivity]) (asynxModels.Event[domain.AgentActivity], error),
	cmd asynxModels.Command[domain.AgentActivity],
) error {
	r := &eventSourced{}
	return r.dispatch(ctx, send, cmd)
}

// MaxOCCAttempts is the retry bound the policy is held to.
const MaxOCCAttempts = maxOCCAttempts
