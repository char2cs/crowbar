package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAsync_ErrorBroadcastsOnEntity(t *testing.T) {
	type call struct {
		id  string
		msg string
	}
	got := make(chan call, 1)
	broadcast := func(wsID string, message string) {
		got <- call{id: wsID, msg: message}
	}

	runAsync(
		context.Background(),
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
	broadcast := func(string, string) { broadcasted <- struct{}{} }
	done := make(chan struct{})

	runAsync(
		context.Background(),
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
		func(string, string) {},
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
