package app

import (
	"context"
	"time"
)

func (c *Container) AbandonWithin(
	ctx context.Context,
	cause error,
	grace time.Duration,
) error {
	return c.abandonWithin(ctx, cause, grace)
}
