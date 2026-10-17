package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureHome_IsAnExistingDirUnderTemp(t *testing.T) {
	h := FixtureHome(t)

	assert.DirExists(t, h.Root())
	assert.Empty(t, TreeSnapshot(t, h))
}

func TestWriteFile_CreatesPrivateParentsAndFile(t *testing.T) {
	h := FixtureHome(t)

	WriteFile(t, h, "state/events/a.log", "hello")

	dirInfo, err := os.Stat(filepath.Join(h.Root(), "state", "events"))
	require.NoError(t, err)
	fileInfo, err := os.Stat(filepath.Join(h.Root(), "state", "events", "a.log"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
}

func TestTreeSnapshot_ListsFilesSortedWithSizes(t *testing.T) {
	h := FixtureHome(t)
	WriteFile(t, h, "state/b", "12")
	WriteFile(t, h, "LAYOUT", "1")

	assert.Equal(t, []string{"LAYOUT 1", "state/b 2"}, TreeSnapshot(t, h))
}

func TestWriteFile_RefusesAPathOutsideTheHome(t *testing.T) {
	h := FixtureHome(t)
	escaped := filepath.Join(filepath.Dir(h.Root()), "escaped")

	for _, rel := range []string{"../escaped", filepath.Join(h.Root(), "abs"), ""} {
		failing := &failureRecorder{TB: t}
		done := make(chan struct{})
		go func() {
			defer close(done)
			WriteFile(failing, h, rel, "x")
		}()
		<-done

		assert.NotEmpty(t, failing.message, "rel %q", rel)
	}

	assert.NoFileExists(t, escaped)
	assert.Empty(t, TreeSnapshot(t, h))
}

type failureRecorder struct {
	testing.TB
	message string
}

func (f *failureRecorder) Fatalf(
	format string,
	args ...any,
) {
	f.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}
