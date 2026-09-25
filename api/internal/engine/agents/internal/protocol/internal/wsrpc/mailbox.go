package wsrpc

import "sync"

// defaultMaxQueued bounds undelivered notifications. A healthy consumer keeps
// the queue near empty; this many means it is wedged, and the connection is
// closed rather than grown without bound.
const defaultMaxQueued = 8192

// mailbox decouples the read loop from the notification consumer: push never
// blocks, deliver feeds out at the consumer's pace.
type mailbox struct {
	mu        sync.Mutex
	queue     []Frame
	finished  bool // the read loop ended: deliver what is queued, then close out
	abandoned bool // Close or overflow: drop what is queued and close out
	overflow  bool
	max       int

	wake chan struct{}
	out  chan Frame
	stop chan struct{}
	once sync.Once
}

func newMailbox() *mailbox {
	return &mailbox{
		max:  defaultMaxQueued,
		wake: make(chan struct{}, 1),
		out:  make(chan Frame),
		stop: make(chan struct{}),
	}
}

// push queues f, or reports false (and abandons the mailbox) when it is full.
func (m *mailbox) push(f Frame) bool {
	m.mu.Lock()
	if len(m.queue) >= m.max {
		m.overflow = true
		m.mu.Unlock()
		m.abandon()
		return false
	}
	m.queue = append(m.queue, f)
	m.mu.Unlock()
	m.signal()
	return true
}

func (m *mailbox) finish() {
	m.mu.Lock()
	m.finished = true
	m.mu.Unlock()
	m.signal()
}

func (m *mailbox) abandon() {
	m.mu.Lock()
	m.abandoned = true
	m.queue = nil
	m.mu.Unlock()
	m.once.Do(func() { close(m.stop) })
}

func (m *mailbox) overflowed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.overflow
}

func (m *mailbox) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// deliver runs for the connection's life and closes out when it is done.
func (m *mailbox) deliver() {
	defer close(m.out)
	for {
		f, ok, done := m.next()
		if done {
			return
		}
		if !ok {
			select {
			case <-m.wake:
			case <-m.stop:
			}
			continue
		}
		select {
		case m.out <- f:
		case <-m.stop:
			return
		}
	}
}

// next pops the head: ok=false when the queue is empty, done=true when there is
// nothing left to ever deliver.
func (m *mailbox) next() (f Frame, ok, done bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.abandoned {
		return Frame{}, false, true
	}
	if len(m.queue) == 0 {
		return Frame{}, false, m.finished
	}
	f = m.queue[0]
	m.queue[0] = Frame{}
	m.queue = m.queue[1:]
	if len(m.queue) == 0 {
		m.queue = nil // release the backing array once drained
	}
	return f, true, false
}
