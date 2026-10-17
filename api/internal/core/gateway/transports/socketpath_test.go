package transports

import (
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

func TestSocketPath_RelativeHomeFails(t *testing.T) {
	t.Setenv(home.EnvVar, "relative/home")

	_, err := SocketPath("unix://")

	require.ErrorIs(t, err, home.ErrRelative)
}
