package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
)

func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}

func TestLedger_AppendTurnsThenRenderOrdered(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	_, err = l.AppendTurn("user", "claude", at, "FIRST")
	require.NoError(t, err)
	_, err = l.AppendTurn("assistant", "claude", at.Add(time.Minute), "SECOND")
	require.NoError(t, err)

	all, err := l.RenderConversation()
	require.NoError(t, err)
	s := string(all)
	require.Less(t, indexOf(s, "FIRST"), indexOf(s, "SECOND"))
	require.Contains(t, s, "user: FIRST")
	require.Contains(t, s, "assistant (claude): SECOND")
}

func TestLedger_AppendTurn_EmptyTextIsNoOp(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	name, err := l.AppendTurn("assistant", "claude", time.Now(), "")
	require.NoError(t, err)
	require.Empty(t, name)
	out, err := l.RenderConversation()
	require.NoError(t, err)
	require.Empty(t, out)
}

// TestOpen_MkdirAllFails_ReturnsError points the ledger dir under an existing
// plain file so MkdirAll fails.
func TestOpen_MkdirAllFails_ReturnsError(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	_, err := ledger.Open(filepath.Join(blocker, "sub"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mkdir")
}

// TestLedger_EntriesReadDirFails_PropagatesToAppendAndReadAll makes the
// ledger dir unreadable after Open succeeds, so entries()'s os.ReadDir call
// fails; both AppendTurn (via nextSeq) and RenderConversation must propagate
// that error.
func TestLedger_EntriesReadDirFails_PropagatesToAppendAndReadAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	_, err = l.AppendTurn("assistant", "claude", time.Now(), "x")
	require.Error(t, err)

	_, err = l.RenderConversation()
	require.Error(t, err)
}

// TestLedger_AppendWriteFileFails_ReadOnlyDir keeps the ledger dir readable
// (so nextSeq's ReadDir still succeeds) but strips write permission, so the
// os.WriteFile inside AppendTurn fails on its own, distinct branch.
func TestLedger_AppendWriteFileFails_ReadOnlyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(dir, 0o500)) // r-x: listable, not writable
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	_, err = l.AppendTurn("assistant", "claude", time.Now(), "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "write")
}

// TestLedger_ReadAllReadFileFails_PerEntryUnreadable makes one already-written
// entry unreadable so RenderConversation's per-entry os.ReadFile fails.
func TestLedger_ReadAllReadFileFails_PerEntryUnreadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	name, err := l.AppendTurn("assistant", "claude", at, "hello")
	require.NoError(t, err)

	blobPath := filepath.Join(dir, name)
	require.NoError(t, os.Chmod(blobPath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(blobPath, 0o644) })

	_, err = l.RenderConversation()
	require.Error(t, err)
	require.Contains(t, err.Error(), "read")
}

// TestLedger_AppendSequenceIncrementsAcrossEntries pins the exact zero-padded
// sequence prefix nextSeq assigns to successive AppendTurn calls.
func TestLedger_AppendSequenceIncrementsAcrossEntries(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	f1, err := l.AppendTurn("assistant", "claude", at, "one")
	require.NoError(t, err)
	f2, err := l.AppendTurn("assistant", "claude", at, "two")
	require.NoError(t, err)
	f3, err := l.AppendTurn("assistant", "claude", at, "three")
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(f1, "00000001-"))
	require.True(t, strings.HasPrefix(f2, "00000002-"))
	require.True(t, strings.HasPrefix(f3, "00000003-"))
}
