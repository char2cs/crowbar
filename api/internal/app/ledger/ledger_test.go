package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestLedger_LastTurnAt_IsWhenTheProviderLastSpoke: the resume path asks the ledger,
// not the runner's conversation history, when a provider "left" — because the ledger is
// the record of what was actually SAID. It is both the gap cut AND the proof that the
// provider's conversation exists on disk at all.
func TestLedger_LastTurnAt_IsWhenTheProviderLastSpoke(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)

	at := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, err = l.AppendTurn("user", "claude", at, "claude's first")
	require.NoError(t, err)
	_, err = l.AppendTurn("assistant", "claude", at.Add(time.Minute), "claude's last")
	require.NoError(t, err)
	// codex spoke AFTER claude left — that is the gap, and it must not move claude's cut.
	_, err = l.AppendTurn("assistant", "codex", at.Add(2*time.Minute), "codex spoke later")
	require.NoError(t, err)

	got, err := l.LastTurnAt("claude")
	require.NoError(t, err)
	assert.Equal(t, at.Add(time.Minute), got)

	// And the gap from that cut is exactly what claude missed.
	gap, err := l.RenderConversationAfter(got)
	require.NoError(t, err)
	assert.Contains(t, string(gap), "codex spoke later")
	assert.NotContains(t, string(gap), "claude's last", "a resumed provider is never re-fed its own turns")
}

// TestLedger_LastTurnAt_ZeroWhenTheProviderNeverSpoke is the phantom-session guard: a
// CLI reports its session id the instant it starts but only WRITES the conversation
// once there is a message, so a provider with no turn here has nothing to resume —
// claude dies on startup with "No conversation found with session ID".
func TestLedger_LastTurnAt_ZeroWhenTheProviderNeverSpoke(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)

	_, err = l.AppendTurn("assistant", "codex", time.Now(), "only codex spoke here")
	require.NoError(t, err)

	got, err := l.LastTurnAt("claude")
	require.NoError(t, err)
	assert.True(t, got.IsZero(), "a provider that never spoke has no conversation to resume")
}

func TestLedger_PageSupportsInitialIncrementalAndOlderWindows(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	for i, text := range []string{"one", "two", "three", "four", "five"} {
		_, err := l.AppendTurn("user", "codex", base.Add(time.Duration(i)*time.Minute), text)
		require.NoError(t, err)
	}

	initial, err := l.Page(0, 0, 2)
	require.NoError(t, err)
	require.Equal(t, []int{4, 5}, []int{initial.Items[0].Sequence, initial.Items[1].Sequence})
	assert.Equal(t, 5, initial.Cursor)
	assert.Equal(t, 4, initial.OldestCursor)
	assert.True(t, initial.HasMore)

	older, err := l.Page(0, initial.OldestCursor, 2)
	require.NoError(t, err)
	require.Equal(t, []int{2, 3}, []int{older.Items[0].Sequence, older.Items[1].Sequence})
	assert.True(t, older.HasMore)

	newer, err := l.Page(3, 0, 1)
	require.NoError(t, err)
	require.Len(t, newer.Items, 1)
	assert.Equal(t, 4, newer.Items[0].Sequence)
	assert.True(t, newer.HasMore)

	last, err := l.Page(5, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, last.Items)
	assert.Zero(t, last.Cursor)
	assert.Zero(t, last.OldestCursor)
	assert.False(t, last.HasMore)
}

func TestLedger_PageRejectsAmbiguousOrInvalidWindow(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)

	_, err = l.Page(1, 2, 10)
	require.Error(t, err)
	_, err = l.Page(0, 0, 0)
	require.Error(t, err)
}

func TestLedger_HasTurnAtOrAfterUsesCurrentConversationBoundary(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	_, err = l.AppendTurn("assistant", "claude", base, "old conversation")
	require.NoError(t, err)
	_, err = l.AppendTurn("user", "codex", base.Add(2*time.Minute), "other provider")
	require.NoError(t, err)
	_, err = l.AppendTurn("user", "claude", base.Add(3*time.Minute), "current conversation")
	require.NoError(t, err)

	got, err := l.HasTurnAtOrAfter("claude", base.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, got)

	got, err = l.HasTurnAtOrAfter("claude", base.Add(4*time.Minute))
	require.NoError(t, err)
	assert.False(t, got)

	got, err = l.HasTurnAtOrAfter("", base)
	require.NoError(t, err)
	assert.False(t, got)
}

func TestLedger_ConcurrentHandlesAllocateUniqueSequences(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	const count = 32
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, err := ledger.Open(dir)
			if err == nil {
				_, err = l.AppendTurn("user", "codex", time.Now(), "message")
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	l, err := ledger.Open(dir)
	require.NoError(t, err)
	turns, err := l.Turns()
	require.NoError(t, err)
	assert.Len(t, turns, count)
	page, err := l.Page(0, 0, count)
	require.NoError(t, err)
	for i, item := range page.Items {
		assert.Equal(t, i+1, item.Sequence)
	}
}
