package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := EventsAt(home)
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Equal(t, filepath.Join(home, "state", "events"), got)
}

func TestStoreAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := StoreAt(home)
	require.NoError(t, err)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestRunsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := RunsAt(home)
	require.NoError(t, err)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestLogsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := LogsAt(home)
	require.NoError(t, err)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestEnsure_RejectsUncreatablePath(t *testing.T) {
	_, err := ensure("/dev/null/cannot")
	assert.Error(t, err)
}
