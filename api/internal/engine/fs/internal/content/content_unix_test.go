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
		_, _ = w.Write(make([]byte, 10))
	}()
	t.Cleanup(func() { <-done })

	_, err := content.ReadWithCap(dir, "growing", 4)
	require.Error(t, err)
	require.ErrorIs(t, err, content.ErrFileTooLarge,
		"a file that grew past the cap after Stat must still be rejected as oversize")
}

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
