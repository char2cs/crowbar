package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// TestLedger_EntriesReadDirFails_PropagatesToAppendTurnAndRenderConversation
// makes the ledger dir unreadable after Open succeeds, so entries()'s
// os.ReadDir call fails; both AppendTurn (via nextSeq) and RenderConversation
// must propagate that error.
func TestLedger_EntriesReadDirFails_PropagatesToAppendTurnAndRenderConversation(t *testing.T) {
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

// TestLedger_RenderConversationReadFileFails_PerEntryUnreadable makes one
// already-written entry unreadable so RenderConversation's per-entry
// os.ReadFile fails.
func TestLedger_RenderConversationReadFileFails_PerEntryUnreadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	l, err := ledger.Open(dir)
	require.NoError(t, err)

	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	name, err := l.AppendTurn("assistant", "claude", at, "hello")
	require.NoError(t, err)

	entryPath := filepath.Join(dir, name)
	require.NoError(t, os.Chmod(entryPath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(entryPath, 0o644) })

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

// TestLedger_RenderConversationAfter_ReturnsOnlyTheGap: a provider resumed into
// its own native session already replays every turn up to the moment it was
// switched out, so it is handed only what was said after that cut — the "while
// you were away" gap. Turns AT the cut are excluded (strictly after), so the
// segment's own last turn can never leak back into its handoff.
func TestLedger_RenderConversationAfter_ReturnsOnlyTheGap(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)

	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, err = l.AppendTurn("user", "codex", base, "before the switch")
	require.NoError(t, err)
	cut := base.Add(time.Minute)
	_, err = l.AppendTurn("assistant", "codex", cut, "exactly at the cut")
	require.NoError(t, err)
	_, err = l.AppendTurn("assistant", "claude", cut.Add(time.Minute), "while codex was away")
	require.NoError(t, err)

	gap, err := l.RenderConversationAfter(cut)
	require.NoError(t, err)

	assert.Contains(t, string(gap), "while codex was away")
	assert.NotContains(t, string(gap), "before the switch")
	assert.NotContains(t, string(gap), "exactly at the cut")
	assert.Contains(t, string(gap), "assistant (claude):")
}

// TestLedger_RenderConversationAfter_NothingNew_IsEmpty: a revive where nothing
// happened while the CLI was gone hands over NO conversation at all, rather than
// an empty wrapper.
func TestLedger_RenderConversationAfter_NothingNew_IsEmpty(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)

	at := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, err = l.AppendTurn("assistant", "claude", at, "said before the CLI died")
	require.NoError(t, err)

	gap, err := l.RenderConversationAfter(at.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, gap)
}

// TestLedger_RenderConversation_ZeroCutRendersEverything guards that the whole
// -ledger path (a provider joining the chat fresh) is unaffected by the gap cut.
func TestLedger_RenderConversation_ZeroCutRendersEverything(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)

	at := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, err = l.AppendTurn("user", "claude", at, "first")
	require.NoError(t, err)
	_, err = l.AppendTurn("assistant", "claude", at.Add(time.Minute), "second")
	require.NoError(t, err)

	all, err := l.RenderConversation()
	require.NoError(t, err)
	assert.Contains(t, string(all), "first")
	assert.Contains(t, string(all), "second")
}
