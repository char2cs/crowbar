package reviewthread_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestChanges_DeliversOnlyToWatchersOfThatWorkspace(t *testing.T) {
	c := reviewthread.NewChanges()
	a, cancelA := c.Watch("ws-a")
	defer cancelA()
	b, cancelB := c.Watch("ws-b")
	defer cancelB()

	c.Notify(domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	require.Equal(t, "t1", (<-a).ID)
	require.Empty(t, b)
}

func TestChanges_FansOutToEveryWatcherOfAWorkspace(t *testing.T) {
	c := reviewthread.NewChanges()
	first, cancel1 := c.Watch("ws-a")
	defer cancel1()
	second, cancel2 := c.Watch("ws-a")
	defer cancel2()

	c.Notify(domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	require.Equal(t, "t1", (<-first).ID)
	require.Equal(t, "t1", (<-second).ID)
}

// A waiter that never drains must not stall the projection that calls Notify.
func TestChanges_NotifyNeverBlocksOnAFullWatcher(t *testing.T) {
	c := reviewthread.NewChanges()
	ch, cancel := c.Watch("ws-a")
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range cap(ch) + 5 {
			c.Notify(domain.ReviewThread{ID: "t", WsID: "ws-a"})
		}
	}()
	<-done

	require.Len(t, ch, cap(ch))
}

func TestChanges_CancelStopsDeliveryAndIsIdempotent(t *testing.T) {
	c := reviewthread.NewChanges()
	ch, cancel := c.Watch("ws-a")

	cancel()
	cancel()
	c.Notify(domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	require.Empty(t, ch)
}
