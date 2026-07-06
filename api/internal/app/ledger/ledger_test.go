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

func TestLedger_AppendThenReadAllOrdered(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	f1, err := l.Append("claude", at, []byte("FIRST"))
	require.NoError(t, err)
	require.Contains(t, f1, "claude")
	_, err = l.Append("codex", at.Add(time.Minute), []byte("SECOND"))
	require.NoError(t, err)

	all, err := l.ReadAll()
	require.NoError(t, err)
	require.Less(t, indexOf(string(all), "FIRST"), indexOf(string(all), "SECOND"))
	require.Contains(t, string(all), "claude")
	require.Contains(t, string(all), "codex")
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
// fails; both Append (via nextSeq) and ReadAll must propagate that error.
func TestLedger_EntriesReadDirFails_PropagatesToAppendAndReadAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	_, err = l.Append("claude", time.Now(), []byte("x"))
	require.Error(t, err)

	_, err = l.ReadAll()
	require.Error(t, err)
}

// TestLedger_AppendWriteFileFails_ReadOnlyDir keeps the ledger dir readable
// (so nextSeq's ReadDir still succeeds) but strips write permission, so the
// os.WriteFile inside Append fails on its own, distinct branch.
func TestLedger_AppendWriteFileFails_ReadOnlyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(dir, 0o500)) // r-x: listable, not writable
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	_, err = l.Append("claude", time.Now(), []byte("x"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "write")
}

// TestLedger_ReadAllReadFileFails_PerEntryUnreadable makes one already-written
// blob unreadable so ReadAll's per-entry os.ReadFile fails.
func TestLedger_ReadAllReadFileFails_PerEntryUnreadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	name, err := l.Append("claude", at, []byte("hello"))
	require.NoError(t, err)

	blobPath := filepath.Join(dir, name)
	require.NoError(t, os.Chmod(blobPath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(blobPath, 0o644) })

	_, err = l.ReadAll()
	require.Error(t, err)
	require.Contains(t, err.Error(), "read")
}

// TestLedger_AppendSequenceIncrementsAcrossEntries pins the exact zero-padded
// sequence prefix nextSeq assigns to successive Append calls.
func TestLedger_AppendSequenceIncrementsAcrossEntries(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	f1, err := l.Append("claude", at, []byte("one"))
	require.NoError(t, err)
	f2, err := l.Append("claude", at, []byte("two"))
	require.NoError(t, err)
	f3, err := l.Append("claude", at, []byte("three"))
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(f1, "00000001-"))
	require.True(t, strings.HasPrefix(f2, "00000002-"))
	require.True(t, strings.HasPrefix(f3, "00000003-"))
}
