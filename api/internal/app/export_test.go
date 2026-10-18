package app

import (
	"context"
	"time"
)

// AbandonWithin exposes the teardown a failed New runs, with the writers' grace
// chosen by the test, so a writer held open across it need not be waited out.
func (c *Container) AbandonWithin(
	ctx context.Context,
	cause error,
	grace time.Duration,
) error {
	return c.abandonWithin(ctx, cause, grace)
}
