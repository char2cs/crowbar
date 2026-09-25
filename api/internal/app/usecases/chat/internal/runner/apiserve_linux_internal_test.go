package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const servePIDFileEnv = "CROWBAR_TEST_SERVE_PID_FILE"

// A daemon that dies without its shutdown (a crash, a second Ctrl-C) must
// not leave its serve processes running: nothing would ever reap them.
func TestForkServeProcess_DiesWithTheDaemon(t *testing.T) {
	if pidFile := os.Getenv(servePIDFileEnv); pidFile != "" {
		serve, err := forkServeProcess([]string{"sleep", "60"})
		if err != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(serve.cmd.Process.Pid)), 0o600)
		os.Exit(0) // the "daemon" dies without killing what it started
	}
	pidFile := filepath.Join(t.TempDir(), "serve.pid")
	daemon := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestForkServeProcess_DiesWithTheDaemon$")
	daemon.Env = append(os.Environ(), servePIDFileEnv+"="+pidFile)
	require.NoError(t, daemon.Run())
	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			_ = syscall.Kill(pid, syscall.SIGKILL) // still ours: it is what failed the test
		}
	})

	require.Eventually(t, func() bool { return !processRunning(pid) },
		5*time.Second, 20*time.Millisecond, "the serve process outlived the daemon that started it")
}
