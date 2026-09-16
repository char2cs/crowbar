package handlers

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyWork records BeginWork/EndWork transitions so the tests can assert the
// working-overlay bracket around the async op.
type spyWork struct {
	mu     sync.Mutex
	begun  []string
	ended  []string
	endCh  chan string
	beganC chan string
}

func newSpyWork() *spyWork {
	return &spyWork{endCh: make(chan string, 4), beganC: make(chan string, 4)}
}

func (s *spyWork) BeginWork(_ context.Context, wsID string) {
	s.mu.Lock()
	s.begun = append(s.begun, wsID)
	s.mu.Unlock()
	s.beganC <- wsID
}

func (s *spyWork) EndWork(_ context.Context, wsID string) {
	s.mu.Lock()
	s.ended = append(s.ended, wsID)
	s.mu.Unlock()
	s.endCh <- wsID
}

func (s *spyWork) IsWorking(string) bool  { return false }
func (s *spyWork) WorkingFor(string) bool { return false }

func (s *spyWork) begunCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.begun)
}

// asyncHandlers builds the bare Handlers that runAsync needs. runAsync touches
// only h.async (the WaitGroup that tracks the detached ops), so the zero value
// of every other dep is fine here.
func asyncHandlers() *Handlers { return &Handlers{} }

func TestRunAsync_ErrorBroadcastsOnEntity(t *testing.T) {
	type call struct {
		id  string
		msg string
	}
	got := make(chan call, 1)
	broadcast := func(_ context.Context, wsID, message string) {
		got <- call{id: wsID, msg: message}
	}

	asyncHandlers().runAsync(
		context.Background(),
		newSpyWork(),
		broadcast,
		"w1",
		func(context.Context) error { return errors.New("boom") },
	)

	// Block on the broadcast itself: its arrival IS the signal. A broadcast that
	// never comes hangs until `go test -timeout`, which names this test.
	c := <-got
	assert.Equal(t, "w1", c.id)
	assert.Equal(t, "boom", c.msg)
}

// TestRunAsync_SuccessDoesNotBroadcast pins that a successful op surfaces no
// error on the entity.
//
// "Nothing was broadcast" is only a sound claim once the producing goroutine is
// provably dead — a sleep never establishes that, it just widens the window in
// which a slow goroutine can hide. WaitAsync blocks until the detached op has
// fully returned (Done is deferred first, so it releases after the recovery
// handler and any broadcast it would make). Once it returns, the broadcaster is
// the only writer and it has exited, so a non-blocking check on the channel is
// exact.
func TestRunAsync_SuccessDoesNotBroadcast(t *testing.T) {
	broadcasted := make(chan struct{}, 1)
	broadcast := func(context.Context, string, string) { broadcasted <- struct{}{} }

	h := asyncHandlers()
	h.runAsync(
		context.Background(),
		newSpyWork(),
		broadcast,
		"w1",
		func(context.Context) error { return nil },
	)

	h.WaitAsync()

	select {
	case <-broadcasted:
		t.Fatal("success path must not broadcast an error")
	default:
	}
}

func TestRunAsync_DetachesFromCancelledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	gotErr := make(chan error, 1)
	asyncHandlers().runAsync(
		parent,
		newSpyWork(),
		func(context.Context, string, string) {},
		"w1",
		func(ctx context.Context) error {
			gotErr <- ctx.Err()
			return nil
		},
	)

	require.NoError(t, <-gotErr, "detached ctx must not inherit parent cancellation")
}

// TestRunAsync_BeginsWorkSynchronously pins the spinner contract: BeginWork
// fires on the request path (before runAsync returns), so the Working=true
// frame races nothing — the client sees the spinner the moment the 202 lands.
func TestRunAsync_BeginsWorkSynchronously(t *testing.T) {
	work := newSpyWork()
	block := make(chan struct{})

	h := asyncHandlers()
	h.runAsync(
		context.Background(),
		work,
		func(context.Context, string, string) {},
		"w1",
		func(context.Context) error {
			<-block
			return nil
		},
	)

	// The op is parked on `block`, so this observation cannot be a lucky race: if
	// BeginWork ran on the goroutine rather than the request path, the count here
	// would still be 0.
	assert.Equal(t, 1, work.begunCount(), "BeginWork must run before runAsync returns")
	close(block)

	assert.Equal(t, "w1", <-work.endCh, "EndWork must be called after fn completed")
}

// TestRunAsync_EndsWorkOnError asserts the overlay is released on the failure
// path too, so a failed op cannot leave a workspace spinning forever.
func TestRunAsync_EndsWorkOnError(t *testing.T) {
	work := newSpyWork()
	order := make(chan string, 2)

	asyncHandlers().runAsync(
		context.Background(),
		work,
		func(context.Context, string, string) { order <- "err" },
		"w1",
		func(context.Context) error { return errors.New("boom") },
	)

	assert.Equal(t, "w1", <-work.endCh, "EndWork must be called on the error path")
	assert.Equal(t, "err", <-order, "broadcastOnErr must be called on the error path")
}

// TestRunAsync_EndsWorkOnPanic asserts a panicking op still releases the
// overlay before the recovered error is surfaced on the entity.
func TestRunAsync_EndsWorkOnPanic(t *testing.T) {
	work := newSpyWork()
	got := make(chan string, 1)

	asyncHandlers().runAsync(
		context.Background(),
		work,
		func(_ context.Context, _, msg string) { got <- msg },
		"w1",
		func(context.Context) error { panic("kaboom") },
	)

	assert.Equal(t, "w1", <-work.endCh, "EndWork must be called on the panic path")
	assert.Contains(t, <-got, "kaboom", "the panic must be surfaced on the entity")
}
