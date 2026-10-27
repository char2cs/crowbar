package watch

import "time"

// burst gathers the events of one debounce window, one merged op per path in
// the order each path first appeared: inotify reports a new file as CREATE then
// WRITE, which together are one "created".
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

// add merges o into name's op and restarts the window.
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

// ready fires once the window has been quiet for debounceDuration.
func (b *burst) ready() <-chan time.Time {
	return b.timer.C
}

// take answers the window's merged events and empties it.
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
