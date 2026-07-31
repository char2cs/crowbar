package internal_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
)

func tempSocket(
	t *testing.T,
) string {
	t.Helper()
	f, err := os.CreateTemp("", "cb-release-*.sock")
	require.NoError(t, err)
	path := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(path))
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

// TestNew_FailureAfterBind_ReleasesTheUnixSocket covers the fd/socket leak on the
// constructor's error path. The listener is bound FIRST, before the engine,
// adapter, app and api layers are wired, so any failure in that wiring used to
// return with the listener still open: an fd held for the life of the process and,
// worse on the unix transport, a live socket file. handleStale dials that leftover
// socket on the NEXT launch, gets an answer from the still-open listener, and
// refuses to start with ErrDaemonRunning — a dead daemon squatting the address.
func TestNew_FailureAfterBind_ReleasesTheUnixSocket(t *testing.T) {
	socket := tempSocket(t)

	container, err := internal.New(
		context.Background(),
		"unix://"+socket,
		nil,
		internal.WithHomeDir("/dev/null/no-such-subdir"),
	)
	if container != nil {
		container.Close()
	}
	require.Error(t, err)

	_, statErr := os.Stat(socket)
	assert.True(t, os.IsNotExist(statErr), "a failed start must not leave its socket bound")
}

// TestNew_LoopbackDisabledByDefault pins the default: without WithLoopbackTCP the
// container owns no TCP listener and publishes no credential.
func TestNew_LoopbackDisabledByDefault(t *testing.T) {
	container, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(container.Close)

	assert.Empty(t, container.LoopbackAddress())
	assert.Empty(t, container.LoopbackCredentialsPath())
}

// TestNew_EmptyLoopbackAddress_IsTheOffSwitch proves WithLoopbackTCP("") — what
// `crowbar serve` passes when neither the flag nor the env var is set — leaves the
// listener off rather than binding a default.
func TestNew_EmptyLoopbackAddress_IsTheOffSwitch(t *testing.T) {
	container, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
		internal.WithLoopbackTCP(""),
	)
	require.NoError(t, err)
	t.Cleanup(container.Close)

	assert.Empty(t, container.LoopbackAddress())
}
