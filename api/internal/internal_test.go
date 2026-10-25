package internal_test

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
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

func TestNew_InvalidHomeDir_ReturnsError(t *testing.T) {
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

// shortHome is a crowbar home under /tmp, so the unix socket fits macOS's
// 104-byte sun_path, and CROWBAR_HOME points there so no default path is real.
func shortHome(
	t *testing.T,
) string {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv(metadata.HomeEnvVar, home)
	return home
}

// writeFileInPlaceOfDir leaves path unusable as a directory, whoever runs the
// test: unlike a permission bit, root cannot write through it.
func writeFileInPlaceOfDir(
	t *testing.T,
	path string,
) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, nil, 0o600))
}

func writeCorruptIdentity(
	t *testing.T,
	tlsDir string,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(tlsDir, 0o700))
	for _, name := range []string{"cert.pem", "key.pem"} {
		require.NoError(t, os.WriteFile(filepath.Join(tlsDir, name), []byte("not a pem"), 0o600))
	}
}

// TestRegression_New_LocalDaemonNeverTouchesTheTLSDir proves a daemon serving
// no TLS — the unix socket alone, or the loopback dev bind — boots whatever
// state its TLS identity directory is in, and never creates one.
func TestRegression_New_LocalDaemonNeverTouchesTheTLSDir(t *testing.T) {
	spoilers := map[string]func(t *testing.T, tlsDir string){
		"absent":    func(*testing.T, string) {},
		"not a dir": writeFileInPlaceOfDir,
		"corrupt":   writeCorruptIdentity,
	}
	for _, host := range []string{"unix", "tcp://127.0.0.1:0"} {
		for name, spoil := range spoilers {
			t.Run(host+" "+name, func(t *testing.T) {
				home := shortHome(t)
				tlsDir := filepath.Join(home, "tls")
				spoil(t, tlsDir)
				bind := host
				if host == "unix" {
					bind = "unix://" + filepath.Join(home, "d.sock")
				}

				c, err := internal.New(context.Background(), bind, nil, internal.WithHomeDir(home))
				require.NoError(t, err)
				t.Cleanup(c.Close)

				if name == "absent" {
					_, statErr := os.Stat(tlsDir)
					assert.ErrorIs(t, statErr, fs.ErrNotExist, "a local daemon must not create a TLS identity")
				}
			})
		}
	}
}

// TestRegression_New_ExposedBindRefusesAnUnloadableIdentity proves a corrupt
// identity is never silently replaced on a network-exposed daemon — every
// paired client pins it — and that the refusal says what replacing it means.
func TestRegression_New_ExposedBindRefusesAnUnloadableIdentity(t *testing.T) {
	home := shortHome(t)
	tlsDir := filepath.Join(home, "tls")
	writeCorruptIdentity(t, tlsDir)

	_, err := internal.New(context.Background(), "tcp://localhost.:0", nil, internal.WithHomeDir(home))

	require.ErrorContains(t, err, "identity cannot be loaded")
	require.ErrorContains(t, err, "identity change")
	cert, readErr := os.ReadFile(filepath.Join(tlsDir, "cert.pem"))
	require.NoError(t, readErr)
	assert.Equal(t, "not a pem", string(cert), "the corrupt identity must be left for the operator, not regenerated")
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
