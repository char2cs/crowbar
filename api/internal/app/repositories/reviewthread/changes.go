package reviewthread

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const watchBuffer = 16

type Changes struct {
	mu       sync.Mutex
	next     int
	watchers map[string]map[int]chan domain.ReviewThread
}

func NewChanges() *Changes {
	return &Changes{watchers: map[string]map[int]chan domain.ReviewThread{}}
}

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
