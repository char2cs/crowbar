//go:build integration

package kit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTB runs the helper's cleanup on demand and keeps its failures.
type recordingTB struct {
	testing.TB
	cleanups []func()
	failures []string
}

func (r *recordingTB) Cleanup(f func())                  { r.cleanups = append(r.cleanups, f) }
func (r *recordingTB) Errorf(format string, args ...any) { r.failures = append(r.failures, format) }

// A grandchild orphaned by its parent's exit is still the test's process.
func TestRequireNoLeakedProcesses_CatchesAnOrphanedGrandchild(t *testing.T) {
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Skip("needs /proc")
	}
	rec := &recordingTB{TB: t}
	RequireNoLeakedProcesses(rec)
	pidFile := filepath.Join(t.TempDir(), "orphan.pid")
	require.NoError(t, exec.CommandContext(t.Context(), "sh", "-c", "sleep 60 >/dev/null 2>&1 & echo $! > "+pidFile).Run())
	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	orphan, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)

	for _, f := range rec.cleanups {
		f()
	}

	assert.NotEmpty(t, rec.failures, "the orphan went unreported")
	assert.Eventually(t, func() bool { return zombie(orphan) }, 5*time.Second, 20*time.Millisecond,
		"the reported orphan is also ended")
}
