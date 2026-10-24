package reviewthread

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// watchBuffer bounds one waiter's backlog. A waiter only needs to learn that
// something changed, so a full buffer already means "wake up".
const watchBuffer = 16

// Changes fans review-thread changes out to in-process waiters, per workspace.
// Notify is the BroadcastFunc the hub projection calls on every event.
type Changes struct {
	mu       sync.Mutex
	next     int
	watchers map[string]map[int]chan domain.ReviewThread
}

func NewChanges() *Changes {
	return &Changes{watchers: map[string]map[int]chan domain.ReviewThread{}}
}

// Notify never blocks: a waiter that is not draining must not stall the projection.
func (c *Changes) Notify(th domain.ReviewThread) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.watchers[th.WsID] {
		select {
		case ch <- th:
		default:
		}
	}
}

// Watch subscribes to changes of one workspace's threads; cancel removes the
// subscription and is safe to call more than once.
func (c *Changes) Watch(wsID string) (<-chan domain.ReviewThread, func()) {
	ch := make(chan domain.ReviewThread, watchBuffer)
	c.mu.Lock()
	id := c.next
	c.next++
	if c.watchers[wsID] == nil {
		c.watchers[wsID] = map[int]chan domain.ReviewThread{}
	}
	c.watchers[wsID][id] = ch
	c.mu.Unlock()

	return ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.watchers[wsID], id)
		if len(c.watchers[wsID]) == 0 {
			delete(c.watchers, wsID)
		}
	}
}
