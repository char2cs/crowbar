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

func TestPaths_Projects(t *testing.T) {
	home := t.TempDir()
	got, err := ProjectsAt(home)
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Equal(t, filepath.Join(home, "projects"), got)

	t.Setenv("HOME", t.TempDir())
	got2, err2 := Projects()
	require.NoError(t, err2)
	assert.NotEmpty(t, got2)
	info2, statErr2 := os.Stat(got2)
	require.NoError(t, statErr2)
	assert.True(t, info2.IsDir())
}

func TestStateAt_IsStateDir(t *testing.T) {
	home := t.TempDir()
	got, err := StateAt(home)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "state"), got)
}

func TestEnsure_RejectsUncreatablePath(t *testing.T) {
	_, err := ensure("/dev/null/cannot")
	assert.Error(t, err)
}

// Tests for the non-At variants using HOME isolation.

func TestEvents_CreatesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Events()
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestStore_CreatesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Store()
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestRuns_CreatesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Runs()
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestLogs_CreatesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Logs()
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}
