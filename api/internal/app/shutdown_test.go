package app_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestApp_Shutdown_BoundedByCtx_DoesNotHangOnStuckReactor(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters, migratetest.NoPostOpen(), v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)

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

	cancel()
	err = <-done

	require.ErrorIs(t, err, context.Canceled,
		"a Shutdown released by cancellation must report the cancellation, not a clean drain")
}

func TestApp_Shutdown_HappyPath_DrainsCleanly(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters, migratetest.NoPostOpen(), v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)

	require.NoError(t, c.Shutdown(context.Background()))
}

func TestApp_Shutdown_LogsAgentToolUsage(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters, migratetest.NoPostOpen(), v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)

	_, _, err = c.Usecases.AgentProvider.DispatchMCP(ctx, "RUN", "forged-token", []byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_chat_title","arguments":{"title":"x"}}}`,
	))
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

func TestApp_Shutdown_GateRefusesNewWorkOnceDraining(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters, migratetest.NoPostOpen(), v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)

	gate := c.Repositories.Drain().Gate
	require.True(t, gate.Enter(), "an open gate admits work")
	gate.Leave()

	require.NoError(t, c.Shutdown(context.Background()))

	require.False(t, gate.Enter(),
		"once the daemon has drained, a late event must NOT be able to spawn a reactor: its "+
			"databases are closed, and the boot sweep is what picks the job back up")
}

func TestApp_Abandon_AWedgedReactorHoldsItNoLongerThanTheGrace(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapters.Close()) })
	c, err := app.New(context.Background(), newEng(t), adapters, migratetest.NoPostOpen(), v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)
	gate := c.Repositories.Drain().Gate
	require.True(t, gate.Enter(), "a reactor that never leaves")
	t.Cleanup(gate.Leave)
	cause := errors.New("the boot failed")
	done := make(chan error, 1)

	go func() { done <- c.AbandonWithin(context.Background(), cause, time.Millisecond) }()

	select {
	case got := <-done:
		require.ErrorIs(t, got, cause)
		require.ErrorIs(t, got, context.DeadlineExceeded, "the wedge, not the teardown finishing, ended the wait")
	case <-time.After(app.WriterDrainGrace / 2):
		t.Fatal("abandon waited on the reactor past the grace")
	}
}
