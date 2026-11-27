//go:build integration

package kit

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

const procTagEnv = "CROWBAR_TEST_PROC_TAG"

const procReapBound = 10 * time.Second

func RequireNoLeakedProcesses(tb testing.TB) {
	tb.Helper()
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		tb.Logf("kit: no /proc, leaked processes not checked: %v", err)
		return
	}
	tag := uuid.NewString()
	tb.Setenv(procTagEnv, tag)
	tb.Cleanup(func() {
		if assert.Eventually(tb, func() bool { return len(taggedProcesses(tag)) == 0 },
			procReapBound, 20*time.Millisecond, "processes started by this test outlived its teardown") {
			return
		}
		leaked := taggedProcesses(tag)
		descs := make([]string, 0, len(leaked))
		for _, pid := range leaked {
			descs = append(descs, describe(pid))
			if err := kill(pid); err != nil {
				tb.Errorf("kill leaked pid %d: %v", pid, err)
			}
		}
		tb.Errorf("leaked (now killed):\n%s", strings.Join(descs, "\n"))
	})
}

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
	cmdline, err := os.ReadFile(filepath.Join("/proc", p, "cmdline")) //nolint:gosec // G304: a /proc entry of a pid this helper listed
	if err != nil {
		return p + ": " + err.Error()
	}
	cwd, err := os.Readlink("/proc/" + p + "/cwd")
	if err != nil {
		return p + ": " + err.Error()
	}
	return p + " (cwd " + cwd + "): " + strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
}

func kill(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
