package testutil

import (
	"os"
	"path/filepath"
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
