//go:build unix

package content_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
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
