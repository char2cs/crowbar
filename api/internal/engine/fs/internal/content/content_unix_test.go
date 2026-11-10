//go:build unix

package content_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// TestRegression_Read_RejectsFileThatGrowsPastCapAfterStat pins the LimitReader
// cap behind Stat. A FIFO stats as size 0 and blocks open until the writer is
// there, so the growth is deterministic without a race.
func TestRegression_Read_RejectsFileThatGrowsPastCapAfterStat(
	t *testing.T,
) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "growing")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer func() { _ = w.Close() }()
		// Comfortably past the 4-byte injected cap regardless of how many bytes
		// the capped read actually consumes.
		_, _ = w.Write(make([]byte, 10))
	}()
	t.Cleanup(func() { <-done })

	_, err := content.ReadWithCap(dir, "growing", 4)
	require.Error(t, err)
	require.ErrorIs(t, err, content.ErrFileTooLarge,
		"a file that grew past the cap after Stat must still be rejected as oversize")
}

// A read-only file must refuse a save, as a direct write would: renaming a temp over it needs only
// the directory's write bit, so the atomic write would otherwise report a success the OS forbids.
func TestWrite_RefusesAReadOnlyFile(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o444))

	err := content.Write(dir, "locked.txt", "changed", "")
	require.ErrorIs(t, err, fs.ErrPermission)
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "original", string(got))

	err = content.WriteAbs(path, "changed", "")
	require.ErrorIs(t, err, fs.ErrPermission)
}
