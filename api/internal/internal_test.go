package internal_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
)

func TestNew_BootsFullStack(t *testing.T) {
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	assert.NotNil(t, c)
}

func TestNew_InvalidHost_ReturnsError(t *testing.T) {
	_, err := internal.New(context.Background(), "bogus://x", nil, internal.WithHomeDir(t.TempDir()))
	assert.Error(t, err)
}

func TestNew_InvalidHomeDir_ReturnsAdapterError(t *testing.T) {
	_, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir("/dev/null/no-such-subdir"),
	)
	assert.Error(t, err)
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

	body := func(resp *http.Response) string {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return string(b)
	}
	resp, err := http.Post(url+"/v0/projects/p/repos/r/chats/runners/seg/hooks/claude/tool_pre",
		"application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	assert.Contains(t, body(resp), "repository not found", "the real router, not the gate, answered")

	resp, err = http.Get(url + "/v0/projects")
	require.NoError(t, err)
	assert.Equal(t, "404 page not found\n", body(resp), "the gate refuses the rest of the API")
}
