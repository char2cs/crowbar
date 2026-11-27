//go:build integration

package kit

import (
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	Main(m)
}

func TestHarness_SocketBootAndRestart(t *testing.T) {
	home := TempHomeForTest(t)

	env1 := BuildEnvAt(t, home)

	sock := env1.SocketPath()
	require.NotEmpty(t, sock, "harness must serve over a Unix socket")
	info, err := os.Stat(sock)
	require.NoError(t, err, "socket file must exist on disk under the temp home")
	require.Equal(t, os.ModeSocket, info.Mode()&os.ModeType,
		"the listener path must be a Unix domain socket, not a TCP port")
	conn, err := net.Dial("unix", sock)
	require.NoError(t, err, "must be able to dial the Unix socket")
	_ = conn.Close()

	projectID := env1.RegisterProject(t, "restart-harness", t.TempDir())

	env1.Close(t)

	env2, err := NewEnvWithHome(home)
	require.NoError(t, err, "NewEnvWithHome must reopen the same home")
	defer env2.Close(t)

	require.NotEqual(t, env1.SocketPath(), env2.SocketPath(),
		"a restart binds its own socket (no stale-socket contention on the same home)")

	resp := env2.GET(t, "/v0/projects")
	RequireStatus(t, resp, http.StatusOK)
	var projects []map[string]any
	DecodeEnvData(t, resp, &projects)

	var found bool
	for _, p := range projects {
		if p["id"] == projectID {
			found = true
			break
		}
	}
	require.True(t, found,
		"restarted env over the same home must read the persisted project (id=%s)", projectID)
}
