package drain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

// A goroutine admitted while a Hold is in force parks at Proceed: it counts as
// admitted (WaitIdle keeps waiting for it) but not as running (WaitRunning
// returns without it), and it goes on the moment the hold is released.
func TestRegression_Gate_HoldParksAdmittedGoroutinesUntilRelease(t *testing.T) {
	g := drain.New()
	g.Hold()
	require.True(t, g.Enter())

	proceeded := make(chan bool, 1)
	go func() {
		defer g.Leave()
		proceeded <- g.Proceed(context.Background())
	}()

	g.WaitRunning(context.Background())
	assert.Equal(t, 1, g.Parked())
	select {
	case <-proceeded:
		t.Fatal("a held goroutine must not proceed before Release")
	default:
	}

	g.Release()
	assert.True(t, <-proceeded)
	g.WaitIdle(context.Background())
	assert.Equal(t, 0, g.Parked())
}

// Without a hold, Proceed is a no-op: the gate admits and lets go as before.
func TestGate_ProceedIsImmediateWhenOpen(t *testing.T) {
	g := drain.New()
	require.True(t, g.Enter())
	defer g.Leave()
	assert.True(t, g.Proceed(context.Background()))
	assert.Equal(t, 0, g.Parked())
}

// A held goroutine whose own deadline expires is told so, and must not run.
func TestGate_ProceedRefusesOnceTheCallerGivesUp(t *testing.T) {
	g := drain.New()
	g.Hold()
	defer g.Release()
	require.True(t, g.Enter())
	defer g.Leave()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, g.Proceed(ctx))
	assert.Equal(t, 0, g.Parked())
}

// WaitRunning is the "no reactor is producing right now" barrier: it returns
// once every admitted goroutine has either left or parked.
func TestGate_WaitRunningReturnsOnceTheLastRunnerLeaves(t *testing.T) {
	g := drain.New()
	require.True(t, g.Enter())
	release := make(chan struct{})
	go func() {
		defer g.Leave()
		<-release
	}()

	done := make(chan struct{})
	go func() {
		g.WaitRunning(context.Background())
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("WaitRunning must block while a goroutine runs")
	default:
	}
	close(release)
	<-done
}
