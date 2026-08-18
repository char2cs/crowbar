package termwait

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// Run drives Sweep on a cadence until ctx is done.
//
// A TICKER, not an event subscription, and that is a deliberate trade rather than
// a shortcut. The terminal engine has no screen-change fan-out that does not also
// have side effects — its only subscription is Attach, which restores a suspended
// session, flushes pending emits and re-primes the differ, none of which an
// observer should cause merely by looking. A pull on a clock leaves the sessions
// it inspects exactly as it found them.
//
// What keeps the clock honest is that the expensive half is change-driven anyway:
// each tick asks the PTY whether its screen has moved since the last look, and
// only renders and scans one that has (see matchScreen). A daemon full of idle
// chats therefore does a few map lookups and an integer compare every couple of
// seconds and nothing else — which is the shape this codebase's own idle-CPU
// regressions taught it to insist on.
//
// The first sweep runs IMMEDIATELY rather than one interval in. A daemon restarting
// under a CLI already parked on a dialog would otherwise show nothing for the first
// interval, which is precisely the state this exists to end.
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
