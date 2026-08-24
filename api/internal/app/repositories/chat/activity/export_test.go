package activity

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func DispatchForTest(
	ctx context.Context,
	send func(context.Context, asynxModels.Command[domain.ChatActivity]) (asynxModels.Event[domain.ChatActivity], error),
	cmd asynxModels.Command[domain.ChatActivity],
) error {
	r := &eventSourced{}
	return r.dispatch(ctx, send, cmd)
}

const MaxOCCAttempts = maxOCCAttempts
