package transports

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

func TestSocketPath_ExplicitPathPassthrough(t *testing.T) {
	got, err := SocketPath("unix:///tmp/explicit.sock")
	require.NoError(t, err)
	require.Equal(t, "/tmp/explicit.sock", got)
}

func TestSocketPath_CrowbarHomeHashed(t *testing.T) {
	t.Setenv(home.EnvVar, t.TempDir())
	h, err := home.Resolve()
	require.NoError(t, err)

	got, err := SocketPath("unix://")
	require.NoError(t, err)
	require.Equal(t, h.Socket(), got)
}

func TestSocketPath_DefaultHomeBindsInsideIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(home.EnvVar, "")
	h, err := home.Resolve()
	require.NoError(t, err)

	got, err := SocketPath("unix://")
	require.NoError(t, err)
	require.Equal(t, h.Socket(), got)
}

// An in-PTY callback's --home always spells the home out, even the default
// one; ipc.NewClient resolves it through SocketPath and must reach the daemon.
func TestSocketPath_SpelledOutDefaultHomeIsNotAnOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME does not set the user home on windows")
	}
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv(home.EnvVar, "")
	unset, err := SocketPath("unix://")
	require.NoError(t, err)

	t.Setenv(home.EnvVar, filepath.Join(userHome, ".crowbar"))
	spelledOut, err := SocketPath("unix://")
	require.NoError(t, err)

	require.Equal(t, filepath.Join(userHome, ".crowbar", "crowbar.sock"), unset)
	require.Equal(t, unset, spelledOut)
}

func TestSocketPath_RelativeHomeFails(t *testing.T) {
	t.Setenv(home.EnvVar, "relative/home")

	_, err := SocketPath("unix://")

	require.ErrorContains(t, err, "must be an absolute path")
}
