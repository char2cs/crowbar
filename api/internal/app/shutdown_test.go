package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
)

// TestApp_Shutdown_BoundedByCtx_DoesNotHangOnStuckReactor asserts the ordered graceful
// shutdown bounds its reactor drain by ctx: a post-commit reactor that never finishes
// cannot wedge Shutdown (unlike quiver's unbounded drainWg.Wait()).
//
// The stimulus is CANCELLATION, not a deadline, and the proof is that Shutdown RETURNS —
// not that it returned quickly. The previous version of this test gave the shutdown a
// 150ms deadline and then asserted `time.Since(start) < 2s`, which is two guesses stacked
// on each other: it could only fail by being slow, and a loaded machine makes it fail on
// code that is perfectly correct. Worse, it could not fail for the reason it exists — a
// genuinely unbounded drain hangs forever, and `go test -timeout` is what catches that,
// with a goroutine dump naming the culprit, which the assertion never could.
func TestApp_Shutdown_BoundedByCtx_DoesNotHangOnStuckReactor(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	// A reactor that is inside the gate and never leaves — exactly the shape of a real
	// post-commit reactor whose work has wedged.
	gate := c.Repositories.Drain().Gate
	require.True(t, gate.Enter(), "the gate must admit work before any drain has begun")
	release := make(chan struct{})
	go func() {
		defer gate.Leave()
		<-release
	}()
	t.Cleanup(func() { close(release) })

	shutdownCtx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- c.Shutdown(shutdownCtx) }()

	// Shutdown is now parked on the stuck reactor. Cancelling is the ONLY thing that can
	// free it, so a Shutdown that returns after this — and only after this — is a Shutdown
	// bounded by its context. An unbounded one never reaches the receive below, and the
	// test binary's own timeout reports it.
	cancel()
	err = <-done

	// It returned, which is the whole contract. It returns an ERROR, and that is not a
	// defect: a shutdown cut short by cancellation could not finish draining, and it says
	// so rather than pretending it went cleanly. Asserting NoError here would be asserting
	// that a cancelled shutdown is a clean one.
	require.ErrorIs(t, err, context.Canceled,
		"a Shutdown released by cancellation must report the cancellation, not a clean drain")
}

// TestApp_Shutdown_HappyPath_DrainsCleanly asserts that with no in-flight reactor work,
// Shutdown drains the reactor gate and every asynx singleton and returns no error.
//
// context.Background() on purpose: with nothing stuck there is nothing to bound, so a
// deadline here would only be a second, quieter way for a slow machine to fail a correct
// shutdown. If this ever fails to drain, it hangs — and `go test -timeout` says so.
func TestApp_Shutdown_HappyPath_DrainsCleanly(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	require.NoError(t, c.Shutdown(context.Background()))
}

// TestApp_Shutdown_LogsAgentToolUsage proves the agent tool counters are actually
// READ somewhere. They were recorded into an instance buried in agenttools.Deps
// with no accessor, no route and no log, so the number the design made a day-one
// requirement ("compliance is a number rather than an impression") was
// unobtainable in a running daemon — the counters may as well not have existed.
//
// Shutdown is the only read, so this is the only place it can be observed.
func TestApp_Shutdown_LogsAgentToolUsage(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	// One call through DispatchMCP, the daemon's only entry to the tool surface.
	// It is REJECTED (forged token) and still counted, which is deliberate: a
	// rejected attempt at a named tool is the most interesting thing this counter
	// can report.
	_, _, err = c.Usecases.Agent.DispatchMCP(ctx, "RUN", "forged-token", []byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_chat_title","arguments":{"title":"x"}}}`))
	require.NoError(t, err)

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(restore) })

	require.NoError(t, c.Shutdown(context.Background()))

	require.Contains(t, logged.String(), "agent tool usage")
	require.Contains(t, logged.String(), "set_chat_title=1/1",
		"the summary must carry the per-tool call and failure counts, not just say it ran")
}

// TestApp_Shutdown_GateRefusesNewWorkOnceDraining pins the property that makes the drain
// converge at all, and that a bare sync.WaitGroup could not give us: once a drain has
// begun the gate ADMITS NOTHING FURTHER, so the outstanding set can only shrink.
//
// This is the half the race detector was complaining about. A reactor Adds on asynx's bus
// goroutine, and shutdown Waits on its own; the PTY kills that graceful shutdown performs
// FIRST commit events, and events wake reactors — so the daemon is at its most eventful in
// the instant it goes quiet. Admitting and draining had to become one critical section.
func TestApp_Shutdown_GateRefusesNewWorkOnceDraining(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	gate := c.Repositories.Drain().Gate
	require.True(t, gate.Enter(), "an open gate admits work")
	gate.Leave() // that work finished; the drain below has nothing to wait for.

	require.NoError(t, c.Shutdown(context.Background()))

	require.False(t, gate.Enter(),
		"once the daemon has drained, a late event must NOT be able to spawn a reactor: its "+
			"databases are closed, and the boot sweep is what picks the job back up")
}
