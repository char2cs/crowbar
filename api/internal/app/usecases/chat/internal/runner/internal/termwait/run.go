package termwait

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

func (d *detector) Run(ctx context.Context, publish Publish) {
	go func() {
		defer safego.Recover("agent.termwait.run")
		ticker := time.NewTicker(d.interval())
		defer ticker.Stop()
		d.Sweep(ctx, publish)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.Sweep(ctx, publish)
			}
		}
	}()
}
