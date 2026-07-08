package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
)

// TestApp_Shutdown_BoundedByCtx_DoesNotHangOnStuckReactor asserts the ordered
// graceful shutdown (spec §3.8, decision 11) bounds its reactor drain by the
// deadline: an in-flight post-commit reactor that never completes cannot wedge
// Shutdown past ctx (unlike quiver's unbounded drainWg.Wait()).
func TestApp_Shutdown_BoundedByCtx_DoesNotHangOnStuckReactor(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	// Simulate an in-flight post-commit reactor: join the SAME shared drain
	// WaitGroup wireCallbacks built (exactly as a real reactor does, decisions
	// 9 + 11) and block it well past the shutdown deadline.
	drain := c.Repositories.Drain()
	release := make(chan struct{})
	drain.WG.Add(1)
	go func() {
		defer drain.WG.Done()
		<-release
	}()
	t.Cleanup(func() { close(release) })

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.Shutdown(shutdownCtx) }()

	// Block on the real shutdown completion: Shutdown's reactor drain is bounded by
	// shutdownCtx (150ms), so it returns on its own — no watchdog timeout needed.
	<-done
	// It returned; assert it did NOT wait out the stuck reactor — the drain wait is
	// bounded by ctx, not unbounded like quiver's drainWg.Wait().
	require.Less(t, time.Since(start), 2*time.Second,
		"Shutdown must return once the ctx deadline lapses, not block on the stuck reactor")
}

// TestApp_Shutdown_HappyPath_DrainsCleanly asserts that with no in-flight reactor
// work, Shutdown drains the reactor gate and every asynx singleton within the
// deadline and returns no error (spec §3.8 steps 2-3).
func TestApp_Shutdown_HappyPath_DrainsCleanly(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c.Shutdown(shutdownCtx))
}
