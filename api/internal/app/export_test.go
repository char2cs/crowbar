package app

import "context"

// Abandon exposes the teardown a failed New runs, so a test can hold a writer
// open across it.
func (c *Container) Abandon(
	ctx context.Context,
	cause error,
) error {
	return c.abandon(ctx, cause)
}
