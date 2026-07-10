package realtime

import "time"

// lingerTimer is the one-shot expiry timer the WatcherManager arms on the 1→0
// refcount transition. Stop reports whether it prevented the callback from
// firing (true) or the callback had already fired / been stopped (false),
// mirroring time.Timer.Stop so *time.Timer satisfies it directly.
type lingerTimer interface {
	Stop() bool
}

// timerFactory builds a lingerTimer that invokes fn after d elapses. It is the
// clock seam: production uses time.AfterFunc; tests inject a controllable timer
// so linger expiry is driven explicitly, never by wall-clock waits.
type timerFactory func(
	d time.Duration,
	fn func(),
) lingerTimer

// realTimer is the production timerFactory backed by time.AfterFunc.
func realTimer(
	d time.Duration,
	fn func(),
) lingerTimer {
	return time.AfterFunc(d, fn)
}
