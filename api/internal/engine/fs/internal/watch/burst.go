package watch

import "time"

type burst struct {
	timer *time.Timer
	ops   map[string]op
	order []string
}

func newBurst() *burst {
	timer := time.NewTimer(debounceDuration)
	if !timer.Stop() {
		<-timer.C
	}
	return &burst{timer: timer, ops: make(map[string]op)}
}

func (b *burst) add(
	name string,
	o op,
) {
	if _, seen := b.ops[name]; !seen {
		b.order = append(b.order, name)
	}
	b.ops[name] |= o
	b.timer.Reset(debounceDuration)
}

func (b *burst) ready() <-chan time.Time {
	return b.timer.C
}

func (b *burst) take() []event {
	events := make([]event, 0, len(b.order))
	for _, name := range b.order {
		events = append(events, event{Name: name, Op: b.ops[name]})
	}
	clear(b.ops)
	b.order = b.order[:0]
	return events
}

func (b *burst) stop() {
	b.timer.Stop()
}
