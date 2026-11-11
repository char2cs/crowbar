package watch_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

// emitBound fails a test whose change never arrives instead of stalling it.
const emitBound = 10 * time.Second

// followFile starts a FileWatcher on path and hands back its emits.
func followFile(
	t *testing.T,
	path string,
) <-chan domain.FileChangeType {
	t.Helper()
	changes := make(chan domain.FileChangeType, 64)
	w := watch.NewFileWatcher(path, func(change domain.FileChangeType) { changes <- change })
	require.NoError(t, w.Start(t.Context()))
	t.Cleanup(w.Stop)
	return changes
}

func nextChange(
	t *testing.T,
	changes <-chan domain.FileChangeType,
) domain.FileChangeType {
	t.Helper()
	select {
	case change := <-changes:
		return change
	case <-time.After(emitBound):
		require.FailNow(t, "no change reported", "within %v", emitBound)
		return ""
	}
}

func writeFile(
	t *testing.T,
	path string,
	text string,
) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

func TestFileWatcher_ReportsAnInPlaceWrite(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.md")
	writeFile(t, a, "v1\n")
	changes := followFile(t, a)

	f, err := os.OpenFile(a, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("v2\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	assert.Equal(t, domain.FileChangeModified, nextChange(t, changes))
}

func TestFileWatcher_AtomicReplaceIsCreated(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	writeFile(t, a, "v1\n")
	changes := followFile(t, a)

	tmp := filepath.Join(dir, ".a.md.tmp")
	writeFile(t, tmp, "v2\n")
	require.NoError(t, os.Rename(tmp, a))

	assert.Equal(t, domain.FileChangeCreated, nextChange(t, changes))
}

func TestFileWatcher_DeleteThenRecreate(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.md")
	writeFile(t, a, "v1\n")
	changes := followFile(t, a)

	require.NoError(t, os.Remove(a))
	assert.Equal(t, domain.FileChangeDeleted, nextChange(t, changes))

	writeFile(t, a, "back\n")
	assert.Equal(t, domain.FileChangeCreated, nextChange(t, changes))
}

func TestFileWatcher_IgnoresSiblingsAndSubdirectories(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	writeFile(t, a, "v1\n")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))
	changes := followFile(t, a)

	writeFile(t, filepath.Join(dir, "b.md"), "b\n")
	writeFile(t, filepath.Join(dir, "a.md.bak"), "bak\n")
	writeFile(t, filepath.Join(dir, "sub", "c.md"), "c\n")
	writeFile(t, a, "v2\n")

	assert.Equal(t, domain.FileChangeModified, nextChange(t, changes), "the first change reported is a.md's own")
}

func TestFileWatcher_WatchesAPathThatDoesNotExistYet(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.md")
	changes := followFile(t, a)

	writeFile(t, a, "new\n")

	assert.Equal(t, domain.FileChangeCreated, nextChange(t, changes))
}

// Deleting the file's directory ends an inotify watch held on it; the file is
// followed again once the directory and the file are made anew. FSEvents may
// not report the file's removal along with its directory's.
func TestFileWatcher_FollowsTheFileAcrossItsDirectoryBeingRecreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	require.NoError(t, os.Mkdir(dir, 0o700))
	a := filepath.Join(dir, "a.md")
	writeFile(t, a, "v1\n")
	changes := followFile(t, a)

	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.Mkdir(dir, 0o700))
	writeFile(t, a, "back\n")

	change := nextChange(t, changes)
	for change == domain.FileChangeDeleted {
		change = nextChange(t, changes)
	}
	assert.Equal(t, domain.FileChangeCreated, change)
}

func TestFileWatcher_EmitsNothingAfterStop(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.md")
	writeFile(t, a, "v1\n")
	stopped := make(chan struct{})
	w := watch.NewFileWatcher(a, func(domain.FileChangeType) {
		select {
		case <-stopped:
			t.Error("emit after Stop returned")
		default:
		}
	})
	require.NoError(t, w.Start(t.Context()))
	writeFile(t, a, "v2\n")

	w.Stop()
	close(stopped)
	writeFile(t, a, "v3\n")
	w.Stop()
}

// A followed file costs a fixed set of descriptors, and churn beside it neither
// grows that set nor reaches the emit.
func TestFileWatcher_SiblingChurnHoldsDescriptorsAndEmitsNothing(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	writeFile(t, a, "v1\n")
	changes := followFile(t, a)
	before := openDescriptors(t)

	for i := range 1000 {
		sibling := filepath.Join(dir, fmt.Sprintf("s%d.md", i))
		writeFile(t, sibling, "x")
		require.NoError(t, os.Remove(sibling))
	}
	writeFile(t, a, "v2\n")

	assert.Equal(t, domain.FileChangeModified, nextChange(t, changes), "the first change reported is a.md's own")
	assert.Equal(t, before, openDescriptors(t))
}
