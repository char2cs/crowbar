package detached_test

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/internal/detached"
)

// Shutdown returns only once the work a handler handed off has returned: the
// daemon must not close its stores under it.
func TestShutdown_WaitsForAnOpStillRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var ops detached.Ops
		release := make(chan struct{})
		ops.Go(context.Background(), "test", func(context.Context) { <-release })

		done := make(chan error, 1)
		go func() { done <- ops.Shutdown(context.Background()) }()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("Shutdown returned while an op was still running")
		default:
		}

		close(release)
		require.NoError(t, <-done)
	})
}

// An op still running when shutdown gives up is cancelled, and the subprocess
// it runs is gone by the time Shutdown returns: nothing outlives the daemon.
func TestShutdown_CancelsWhatOutlivesItsBudgetAndLeavesNoChild(t *testing.T) {
	var ops detached.Ops
	pid := make(chan int, 1)
	ops.Go(context.Background(), "test", func(ctx context.Context) {
		cmd := exec.CommandContext(ctx, "sleep", "600")
		if err := cmd.Start(); err != nil {
			pid <- 0
			return
		}
		pid <- cmd.Process.Pid
		_ = cmd.Wait()
	})
	child := <-pid
	require.NotZero(t, child)

	budget, cancel := context.WithCancel(context.Background())
	cancel()
	err := ops.Shutdown(budget)

	require.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, syscall.Kill(child, 0), syscall.ESRCH, "the op's subprocess outlived Shutdown")
}

// A handler's request ctx is cancelled once its 202 is flushed; the op it
// handed off must not be.
func TestGo_TheOpOutlivesTheRequest(t *testing.T) {
	var ops detached.Ops
	request, cancel := context.WithCancel(context.Background())
	opCtx := make(chan context.Context, 1)
	proceed := make(chan struct{})
	ops.Go(request, "test", func(ctx context.Context) {
		opCtx <- ctx
		<-proceed
	})
	cancel()
	ctx := <-opCtx

	require.NoError(t, ctx.Err())
	close(proceed)
	require.NoError(t, ops.Shutdown(context.Background()))
}
