package tools

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func ReviewWaitTimeoutForTest(seconds int) time.Duration { return waitTimeout(seconds) }

func WaitForReviewCommentsForTest(
	ctx context.Context,
	deps Deps,
	wsID string,
	timeout time.Duration,
) (string, error) {
	return waitForReviewComments(ctx, deps, Caller{Workspace: domain.Workspace{ID: wsID}}, timeout)
}
