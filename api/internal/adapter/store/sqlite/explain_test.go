package sqlite_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// openAndWrite opens the database at path and writes to it, as a store does
// on its first use, returning the driver's error.
func openAndWrite(t *testing.T, path string) error {
	t.Helper()
	db, err := sqlite.OpenDB(path)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() { require.NoError(t, sqlDB.Close()) }()
	return db.Exec("CREATE TABLE IF NOT EXISTS t (x)").Error
}

func TestExplain_NamesReadOnlyFullAndDamagedInPlainWords(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.db")
	require.NoError(t, os.WriteFile(garbage, []byte("this is not a sqlite database, not even a little"), 0o600))
	damaged := openAndWrite(t, garbage)
	require.Error(t, damaged)

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"damaged", damaged, "damaged"},
		{"disk full", fmt.Errorf("write: %w", syscall.ENOSPC), "full"},
		{"read-only file system", fmt.Errorf("open: %w", syscall.EROFS), "read-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words, ok := sqlite.Explain(tc.err)
			assert.True(t, ok)
			assert.Contains(t, words, tc.want)
		})
	}
	_, ok := sqlite.Explain(errors.New("boom"))
	assert.False(t, ok, "anything else keeps its own text")
}

func TestExplain_ADatabaseInAFolderItCannotWriteIsReadOnly(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	dir := filepath.Join(t.TempDir(), "locked")
	require.NoError(t, os.Mkdir(dir, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o700)) })

	err := openAndWrite(t, filepath.Join(dir, "x.db"))

	require.Error(t, err)
	words, ok := sqlite.Explain(err)
	assert.True(t, ok, err.Error())
	assert.Contains(t, words, "read-only")
}

// A refused file names itself: the folder is rarely read-only as a whole, and
// without the path a user cannot find the one file another user owns.
func TestExplain_APermissionRefusalNamesThePath(t *testing.T) {
	refused := fmt.Errorf("owner-only: %w", &fs.PathError{Op: "chmod", Path: "/home/u/.crowbar/x.json", Err: syscall.EPERM})

	words, ok := sqlite.Explain(refused)

	assert.True(t, ok)
	assert.Contains(t, words, "may not write")
	assert.Contains(t, words, "/home/u/.crowbar/x.json")
}
