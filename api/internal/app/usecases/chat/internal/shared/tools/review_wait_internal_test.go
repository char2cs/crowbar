package tools

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReviewWaitTimeoutForTest exposes the clamp, so the bounds are asserted from the
// production function rather than copied numbers.
func ReviewWaitTimeoutForTest(seconds int) time.Duration { return waitTimeout(seconds) }

// WaitForReviewCommentsForTest runs the wait with an explicit deadline, so the
// timeout path is reachable without waiting out the one-second minimum.
func WaitForReviewCommentsForTest(
	ctx context.Context,
	deps Deps,
	wsID string,
	timeout time.Duration,
) (string, error) {
	return waitForReviewComments(ctx, deps, Caller{Workspace: domain.Workspace{ID: wsID}}, timeout)
}
