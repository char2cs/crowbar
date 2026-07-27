package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/perf"
)

func TestLockRepoRead_RecordsWaitAndHold(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	// New() — NOT &engine{}. repoMutex resolves the git common dir through
	// e.exec (engine.go:56), so an engine with a nil exec func panics here.
	e := New().(*engine)
	unlock := e.lockRepoRead(context.Background(), t.TempDir())
	unlock()

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "lock.read.wait")
	assert.Contains(t, names, "lock.read.hold")
}

func TestLockRepo_RecordsWaitAndHold(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	e := New().(*engine)
	unlock := e.lockRepo(context.Background(), t.TempDir())
	unlock()

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "lock.write.wait")
	assert.Contains(t, names, "lock.write.hold")
}
