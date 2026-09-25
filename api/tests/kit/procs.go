//go:build integration

package kit

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// procTagEnv is inherited by every process a test starts, however deep: a
// grandchild orphaned onto init still carries it, where its parent link does not.
const procTagEnv = "CROWBAR_TEST_PROC_TAG"

// procReapBound is how long a teardown may take to see its processes exit
// (codex's app-server is asked to stop gracefully, then waited on).
const procReapBound = 10 * time.Second

// RequireNoLeakedProcesses fails t if any process started during it — child,
// grandchild or orphan — is still running when every other cleanup is done.
// Call it before anything is started, so its cleanup runs last. Leaked
// processes are killed so one failure does not pile up behind the next test.
// It reads /proc; where there is none it checks nothing.
func RequireNoLeakedProcesses(t testing.TB) {
	t.Helper()
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Logf("kit: no /proc, leaked processes not checked: %v", err)
		return
	}
	tag := uuid.NewString()
	t.Setenv(procTagEnv, tag)
	t.Cleanup(func() {
		// A process that is exiting is still listed until it is reaped.
		if assert.Eventually(t, func() bool { return len(taggedProcesses(tag)) == 0 },
			procReapBound, 20*time.Millisecond, "processes started by this test outlived its teardown") {
			return
		}
		leaked := taggedProcesses(tag)
		descs := make([]string, 0, len(leaked))
		for _, pid := range leaked {
			descs = append(descs, describe(pid))
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		t.Errorf("leaked (now killed):\n%s", strings.Join(descs, "\n"))
	})
}

// taggedProcesses lists the live (non-zombie) processes, other than this one,
// whose environment carries tag.
func taggedProcesses(tag string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	needle := []byte(procTagEnv + "=" + tag + "\x00")
	self := os.Getpid()
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self || zombie(pid) {
			continue
		}
		env, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err == nil && bytes.Contains(env, needle) {
			out = append(out, pid)
		}
	}
	return out
}

// zombie reports whether pid has exited and only awaits its parent's reap.
func zombie(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	s := string(stat)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	return len(fields) == 0 || fields[0] == "Z" || fields[0] == "X"
}

func describe(pid int) string {
	p := strconv.Itoa(pid)
	cmdline, _ := os.ReadFile("/proc/" + p + "/cmdline")
	cwd, _ := os.Readlink("/proc/" + p + "/cwd")
	return p + " (cwd " + cwd + "): " + strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
}
