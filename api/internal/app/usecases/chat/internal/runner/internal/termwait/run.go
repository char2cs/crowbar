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

func (d *detector) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}
