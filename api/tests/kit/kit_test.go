//go:build integration

package kit

import (
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMain is the kit package's own integration entry point (silence logs, gin
// test mode) so the harness self-test can run under `-tags integration`.
func TestMain(m *testing.M) {
	Main(m)
}

// TestHarness_SocketBootAndRestart is the Task 16 harness self-test (spec §5):
//
//	BuildEnv boots and serves the real adapter+app+api stack over a Unix domain
//	socket under a WithHomeDir-isolated temp home; a SECOND Env opened over the
//	SAME home via the restart primitive NewEnvWithHome reads back the durable
//	read-model state the first env persisted.
//
// This proves the three harness invariants Task 17 builds on: (1) real serving
// over a short-path Unix socket (macOS 104-byte sun_path), (2) WithHomeDir
// isolation, and (3) restart = a second env over the same home dir.
func TestHarness_SocketBootAndRestart(t *testing.T) {
	home := TempHomeForTest(t)

	// --- env1: boots + serves the real stack over a Unix socket. ---
	env1 := BuildEnvAt(t, home)

	sock := env1.SocketPath()
	require.NotEmpty(t, sock, "harness must serve over a Unix socket")
	info, err := os.Stat(sock)
	require.NoError(t, err, "socket file must exist on disk under the temp home")
	require.Equal(t, os.ModeSocket, info.Mode()&os.ModeType,
		"the listener path must be a Unix domain socket, not a TCP port")
	// Serving is genuinely over that socket: a raw unix dial connects.
	conn, err := net.Dial("unix", sock)
	require.NoError(t, err, "must be able to dial the Unix socket")
	_ = conn.Close()

	// Persist state through the real HTTP+WS stack (create + broadcast a project).
	projectID := env1.RegisterProject(t, "restart-harness", t.TempDir())

	// Graceful teardown releases the socket file + DB handles so a second env can
	// reopen the same home.
	env1.Close(t)

	// --- env2: restart primitive over the SAME home reads persisted state. ---
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
