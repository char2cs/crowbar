package transports

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// tempSocketPath returns a short unique socket path safe on macOS, where the
// sun_path limit (104 bytes) makes long t.TempDir() paths overflow.
func tempSocketPath(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "cb-*.sock")
	require.NoError(t, err)
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func TestNewSocket_ExplicitPath(t *testing.T) {
	sockPath := tempSocketPath(t)
	l, err := NewSocket("unix://" + sockPath)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
	_, statErr := os.Stat(sockPath)
	assert.NoError(t, statErr, "socket file should exist after NewSocket")
}

func TestNewSocket_DefaultPath_UsesHome(t *testing.T) {
	// A short prefix keeps ~/.crowbar/crowbar.sock under the sun_path limit.
	userHome, err := os.MkdirTemp("", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(userHome) })
	t.Setenv("HOME", userHome)
	t.Setenv(home.EnvVar, "")
	l, err := NewSocket("unix://")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
	_, statErr := os.Stat(filepath.Join(userHome, ".crowbar", "crowbar.sock"))
	assert.NoError(t, statErr, "socket should be created at ~/.crowbar/crowbar.sock")
}

// The binder, not the resolver, creates the socket's directory.
func TestNewSocket_CreatesTheSocketDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "nested", "s.sock")

	l, err := NewSocket("unix://" + sockPath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, l.Close()) })

	_, statErr := os.Stat(sockPath)
	assert.NoError(t, statErr, "socket file should exist in the directory NewSocket created")
}

func TestNewSocket_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits not enforced on Windows")
	}
	sockPath := tempSocketPath(t)
	l, err := NewSocket("unix://" + sockPath)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })

	info, err := os.Stat(sockPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestNewSocket_StaleSocket_IsReclaimed(t *testing.T) {
	sockPath := tempSocketPath(t)

	// Simulate a crash: bind then leave the file behind on close.
	stale, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())

	l, err := NewSocket("unix://" + sockPath)
	require.NoError(t, err, "a stale socket should be reclaimed, not rejected")
	t.Cleanup(func() { l.Close() })

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err, "the reclaimed socket should accept connections")
	_ = conn.Close()
}

func TestNewSocket_DaemonAlreadyRunning(t *testing.T) {
	sockPath := tempSocketPath(t)

	active, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer active.Close()
	go func() {
		for {
			conn, err := active.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	_, err = NewSocket("unix://" + sockPath)
	assert.ErrorIs(t, err, ErrDaemonRunning)
}

func TestNewSocket_ListenError(t *testing.T) {
	// A path longer than sun_path can be created as a directory but not bound.
	tooLong := filepath.Join(t.TempDir(), strings.Repeat("x", 120), "crowbar.sock")
	_, err := NewSocket("unix://" + tooLong)
	assert.Error(t, err)
}

func TestNewSocket_ChmodError_CleansUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is not applied on Windows")
	}
	sockPath := tempSocketPath(t)

	prev := chmod
	chmod = func(string, os.FileMode) error { return os.ErrPermission }
	t.Cleanup(func() { chmod = prev })

	_, err := NewSocket("unix://" + sockPath)
	assert.ErrorIs(t, err, os.ErrPermission)

	_, statErr := os.Stat(sockPath)
	assert.True(t, os.IsNotExist(statErr), "socket file should be cleaned up after chmod failure")
}

func TestSocketListener_Close_RemovesFile(t *testing.T) {
	sockPath := tempSocketPath(t)

	l, err := NewSocket("unix://" + sockPath)
	require.NoError(t, err)
	require.NoError(t, l.Close())

	_, err = os.Stat(sockPath)
	assert.True(t, os.IsNotExist(err), "socket file should be removed after Close")
}

// TestNewSocket_CrowbarHomeOverride_BindsShortPath proves an override home too
// deep for sun_path still yields a bindable socket.
func TestNewSocket_CrowbarHomeOverride_BindsShortPath(t *testing.T) {
	deep := filepath.Join(t.TempDir(), strings.Repeat("deep-segment/", 12), ".crowbar")
	t.Setenv(home.EnvVar, deep)

	l, err := NewSocket("unix://")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	assert.Greater(t, 104, len(l.Addr().String()), "socket path must fit sun_path")
}
