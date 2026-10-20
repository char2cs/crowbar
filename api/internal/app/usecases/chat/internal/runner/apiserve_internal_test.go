package runner

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Serve's stdin and stdout are the connection: what is written to one is what
// the process reads, and what it prints is what the other returns.
func TestForkServeProcess_ItsStdioIsTheConnection(t *testing.T) {
	serve, stdio, err := forkServeProcess([]string{"sh", "-c", `read line; echo "got $line"`})
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })
	defer serve.kill()

	_, err = stdio.In.Write([]byte("hello\n"))
	require.NoError(t, err)
	line, err := bufio.NewReader(stdio.Out).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "got hello\n", line)
}

// A serve can be a wrapper around the real binary: killing only the wrapper
// orphaned the process it had started, which ran on forever.
func TestForkServeProcess_KillEndsWhatTheProcessStarted(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	serve, stdio, err := forkServeProcess([]string{"sh", "-c", "sleep 60 & echo $! > " + pidFile + "; wait"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })
	var grandchild int
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil || !strings.HasSuffix(string(raw), "\n") {
			return false
		}
		grandchild, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)

	serve.kill()

	require.Eventually(t, func() bool { return !processRunning(grandchild) },
		5*time.Second, 20*time.Millisecond, "the serve process's own child outlived it")
}

// processRunning reports whether pid exists and is not a zombie awaiting reap.
func processRunning(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return syscall.Kill(pid, 0) == nil
	}
	s := string(stat)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	return len(fields) > 0 && fields[0] != "Z"
}

func TestForkServeProcess_EmptyArgvIsAnError(t *testing.T) {
	_, _, err := forkServeProcess(nil)
	assert.Error(t, err)
}
