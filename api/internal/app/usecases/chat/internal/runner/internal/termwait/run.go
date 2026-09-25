package termwait

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// Run sweeps immediately and then on the interval — but only while there is
// something to sweep. A sweep that finds no live runner parks the loop until
// Wake (a runner was recorded live), so an idle daemon does no periodic work:
// no timer, no read transaction every interval (spec §6a).
func (d *detector) Run(ctx context.Context, publish Publish) {
	go func() {
		defer safego.Recover("agent.termwait.run")
		ticker := time.NewTicker(d.interval())
		defer ticker.Stop()
		active := d.sweep(ctx, publish)
		for {
			if !active {
				ticker.Stop()
				select {
				case <-ctx.Done():
					return
				case <-d.wake:
				}
				ticker.Reset(d.interval())
				active = d.sweep(ctx, publish)
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				active = d.sweep(ctx, publish)
			}
		}
	}()
}

// Wake resumes a parked Run loop. Non-blocking, and safe to call whether or
// not the loop is parked or running at all.
func (d *detector) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}
