package answerdesk

import "time"

// WithBudgetClock runs every relay's budget out when after's channel fires.
func WithBudgetClock(after func(time.Duration) <-chan time.Time) Option {
	return func(d *Desk) { d.after = after }
}
