//go:build integration

package kit

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// RequireNoChildProcesses fails t if the test process still has a child: after
// a daemon's teardown returns, nothing it started may still be running. It
// reads /proc, so it checks nothing where there is none.
func RequireNoChildProcesses(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Logf("kit: no /proc, child processes not checked: %v", err)
		return
	}
	self := strconv.Itoa(os.Getpid())
	var children []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if parentOf(e.Name()) == self {
			cmdline, _ := os.ReadFile("/proc/" + e.Name() + "/cmdline")
			children = append(children, e.Name()+": "+strings.ReplaceAll(string(cmdline), "\x00", " "))
		}
	}
	assert.Empty(t, children, "processes the daemon started outlived its teardown")
}

// parentOf reads pid's parent from /proc/<pid>/stat, whose second field (the
// command name) may itself contain spaces and parentheses.
func parentOf(pid string) string {
	stat, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return ""
	}
	s := string(stat)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}
