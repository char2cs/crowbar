package internal_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestNew_BootsFullStack(t *testing.T) {
	dir := t.TempDir()
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(dir),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	assert.NotNil(t, c)
	assert.Equal(t, dir, c.Home())
}

// A callback is handed the socket the daemon bound, so the daemon must say which
// one that is: the home as given, even when it is not clean, and none for TCP.
func TestNew_ReportsTheSocketItBound(t *testing.T) {
	t.Setenv(home.EnvVar, t.TempDir()+"/x/")
	h, err := home.Resolve()
	require.NoError(t, err)

	c, err := internal.New(context.Background(), "unix://", nil)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	assert.Equal(t, h.Socket(), c.Socket())
	assert.FileExists(t, c.Socket())
}

func TestNew_ReportsNoSocketOverTCP(t *testing.T) {
	c, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	assert.Empty(t, c.Socket())
}

func TestNew_InvalidHost_ReturnsError(t *testing.T) {
	_, err := internal.New(context.Background(), "bogus://x", nil, internal.WithHomeDir(t.TempDir()))
	assert.Error(t, err)
}

func TestNew_InvalidHomeDir_ReturnsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(filepath.Join(file, "home")),
	)
	assert.ErrorContains(t, err, "internal: migrate")
}

// The daemon holds its home while it runs, so a second one is refused before
// it opens a store, and the home is free again once the first one closes.
func TestNew_RefusesASecondDaemonOnTheSameHome(t *testing.T) {
	dir := t.TempDir()
	first, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(dir))
	require.NoError(t, err)

	_, err = internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(dir))
	require.ErrorIs(t, err, migrate.ErrHomeLocked)

	first.Close()
	again, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(dir))
	require.NoError(t, err)
	again.Close()
}

func TestNew_RunsTheLegacyStepsBeforeServing(t *testing.T) {
	h := testutil.FixtureHome(t)

	c, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(h.Root()))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	applied := migratetest.Applied(t, h)
	for step := 1; step <= 5; step++ {
		assert.Len(t, applied[step], 1, "step %d", step)
	}
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(home.EnvVar, "")
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	assert.NotNil(t, c)
}

func TestRun_CancelledContext_Returns(t *testing.T) {
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = c.Run(ctx)
	assert.NoError(t, err)
}

// A vendor CLI's HTTP hook reaches the real router over loopback TCP, and nothing
// else of the API is served on that port.
func TestRun_ServesOnlyTheRunnerHookRouteOnALoopbackPort(t *testing.T) {
	home := t.TempDir()
	c, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	url, err := hookserver.Read(home)
	require.NoError(t, err, "the daemon publishes where hooks are served")
	require.True(t, strings.HasPrefix(url, "http://127.0.0.1:"), url)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	fetch := func(method, target string, payload io.Reader) string {
		req, err := http.NewRequestWithContext(ctx, method, target, payload)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(b)
	}
	assert.Contains(t, fetch(http.MethodPost, url+"/v0/projects/p/repos/r/chats/runners/seg/hooks/claude/tool_pre", strings.NewReader("{}")),
		"repository not found", "the real router, not the gate, answered")
	assert.Equal(t, "404 page not found\n", fetch(http.MethodGet, url+"/v0/projects", nil),
		"the gate refuses the rest of the API")
}
