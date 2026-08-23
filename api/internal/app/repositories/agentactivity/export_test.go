package agentactivity

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func DispatchForTest(
	ctx context.Context,
	send func(context.Context, asynxModels.Command[domain.AgentActivity]) (asynxModels.Event[domain.AgentActivity], error),
	cmd asynxModels.Command[domain.AgentActivity],
) error {
	r := &eventSourced{}
	return r.dispatch(ctx, send, cmd)
}

const MaxOCCAttempts = maxOCCAttempts
