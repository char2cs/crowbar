package reactors

import "time"

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
