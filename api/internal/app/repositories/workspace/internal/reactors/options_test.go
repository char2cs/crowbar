package reactors

import "time"

// WithRetryBackoff sets the exponential backoff between failed purge attempts:
// the first retry waits base, each next one doubles, capped at maxDelay.
// Non-positive values are ignored.
func WithRetryBackoff(
	base time.Duration,
	maxDelay time.Duration,
) Opt {
	return func(r *deleteReactor) {
		if base > 0 {
			r.retryBase = base
		}
		if maxDelay > 0 {
			r.retryCap = maxDelay
		}
	}
}
