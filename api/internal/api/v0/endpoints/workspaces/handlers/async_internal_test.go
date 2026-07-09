package handlers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

func (s *spyWork) IsWorking(string) bool { return false }

func (s *spyWork) begunCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.begun)
}

func TestRunAsync_ErrorBroadcastsOnEntity(t *testing.T) {
	type call struct {
		id  string
		msg string
	}
	got := make(chan call, 1)
	broadcast := func(_ context.Context, wsID, message string) {
		got <- call{id: wsID, msg: message}
	}

	runAsync(
		context.Background(),
		newSpyWork(),
		broadcast,
		"w1",
		func(context.Context) error { return errors.New("boom") },
	)

	select {
	case c := <-got:
		assert.Equal(t, "w1", c.id)
		assert.Equal(t, "boom", c.msg)
	case <-time.After(time.Second):
		t.Fatal("expected broadcastOnErr to be called")
	}
}

func TestRunAsync_SuccessDoesNotBroadcast(t *testing.T) {
	broadcasted := make(chan struct{}, 1)
	broadcast := func(context.Context, string, string) { broadcasted <- struct{}{} }
	done := make(chan struct{})

	runAsync(
		context.Background(),
		newSpyWork(),
		broadcast,
		"w1",
		func(context.Context) error {
			close(done)
			return nil
		},
	)

	<-done
	select {
	case <-broadcasted:
		t.Fatal("success path must not broadcast an error")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRunAsync_DetachesFromCancelledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	gotErr := make(chan error, 1)
	runAsync(
		parent,
		newSpyWork(),
		func(context.Context, string, string) {},
		"w1",
		func(ctx context.Context) error {
			gotErr <- ctx.Err()
			return nil
		},
	)

	select {
	case err := <-gotErr:
		require.NoError(t, err, "detached ctx must not inherit parent cancellation")
	case <-time.After(time.Second):
		t.Fatal("fn did not run")
	}
}

// TestRunAsync_BeginsWorkSynchronously pins the spinner contract: BeginWork
// fires on the request path (before runAsync returns), so the Working=true
// frame races nothing — the client sees the spinner the moment the 202 lands.
func TestRunAsync_BeginsWorkSynchronously(t *testing.T) {
	work := newSpyWork()
	block := make(chan struct{})

	runAsync(
		context.Background(),
		work,
		func(context.Context, string, string) {},
		"w1",
		func(context.Context) error {
			<-block
			return nil
		},
	)

	assert.Equal(t, 1, work.begunCount(), "BeginWork must run before runAsync returns")
	close(block)

	select {
	case id := <-work.endCh:
		assert.Equal(t, "w1", id)
	case <-time.After(time.Second):
		t.Fatal("EndWork was not called after fn completed")
	}
}

// TestRunAsync_EndsWorkOnError asserts the overlay is released on the failure
// path too, so a failed op cannot leave a workspace spinning forever.
func TestRunAsync_EndsWorkOnError(t *testing.T) {
	work := newSpyWork()
	order := make(chan string, 2)

	runAsync(
		context.Background(),
		work,
		func(context.Context, string, string) { order <- "err" },
		"w1",
		func(context.Context) error { return errors.New("boom") },
	)

	select {
	case id := <-work.endCh:
		assert.Equal(t, "w1", id)
	case <-time.After(time.Second):
		t.Fatal("EndWork was not called on the error path")
	}
	select {
	case <-order:
	case <-time.After(time.Second):
		t.Fatal("broadcastOnErr was not called")
	}
}

// TestRunAsync_EndsWorkOnPanic asserts a panicking op still releases the
// overlay before the recovered error is surfaced on the entity.
func TestRunAsync_EndsWorkOnPanic(t *testing.T) {
	work := newSpyWork()
	got := make(chan string, 1)

	runAsync(
		context.Background(),
		work,
		func(_ context.Context, _, msg string) { got <- msg },
		"w1",
		func(context.Context) error { panic("kaboom") },
	)

	select {
	case id := <-work.endCh:
		assert.Equal(t, "w1", id)
	case <-time.After(time.Second):
		t.Fatal("EndWork was not called on the panic path")
	}
	select {
	case msg := <-got:
		assert.Contains(t, msg, "kaboom")
	case <-time.After(time.Second):
		t.Fatal("panic was not surfaced on the entity")
	}
}
