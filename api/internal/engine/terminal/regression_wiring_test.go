package terminal_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// readBufHeader returns the first line (the CRWB1 header) of <dir>/<sid>.buf, or "" if absent.
func readBufHeader(t *testing.T, dir, sid string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, sid+".buf"))
	if err != nil {
		return ""
	}
	if nl := bytes.IndexByte(data, '\n'); nl >= 0 {
		return string(data[:nl])
	}
	return string(data)
}

// TestRegression_IdleSessionNoCadenceWrite guards the §8.4 write-skip signal: an idle session
// (no new PTY output since the last flush) causes ZERO cadence disk I/O. flushSessionOnce
// short-circuits on Snapshot()'s changed==false before WriteBuf and saveMeta, so a 10 s cadence
// tick over an idle session performs no WriteBuf and no saveMeta — preventing the
// 100-sessions-every-10s write regression. The paired assertion proves one byte of new output
// flips changed back to true and the next tick writes again.
//
// The "hooked persistence layer" is the fakeMetaStore (counts saveMeta) plus the .buf file
// itself: we delete the committed .buf and assert an idle tick does NOT recreate it (no
// WriteBuf), then assert a post-output tick DOES recreate it.
func TestRegression_IdleSessionNoCadenceWrite(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // drive maintenance manually; no ticker races
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Block until the shell is parked at its prompt — the only point at which "the session
	// has no output still in flight" is a fact rather than an assumption. The whole test is a
	// negative assertion about an IDLE session, so establishing genuine idleness is not a
	// preliminary here: it is the hypothesis.
	sid := newReadyShell(t, eng, "ws-idle", dir)

	// First cadence flush: dirty==true → .buf written + meta saved, and the dirty bit is
	// consumed. Nothing can set it again while the shell sits at its prompt, so one flush is
	// enough to reach the clean dirty==false state the assertion needs (this used to take a
	// second speculative flush sandwiched between two settle windows).
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	require.True(t, bufExists(store.dir, sid), "first cadence flush must write the .buf")

	// Delete the committed .buf and snapshot the meta-save count: a subsequent IDLE tick must
	// recreate NEITHER (changed==false → flushSessionOnce returns before WriteBuf+saveMeta).
	require.NoError(t, os.Remove(filepath.Join(store.dir, sid+".buf")))
	savesBefore := countSavedForSession(store, sid)

	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.False(t, bufExists(store.dir, sid),
		"an idle (unchanged) cadence tick must perform NO WriteBuf — the deleted .buf must stay gone")
	assert.Equal(t, savesBefore, countSavedForSession(store, sid),
		"an idle (unchanged) cadence tick must perform NO saveMeta")

	// Paired: new output flips changed back to true; the next tick writes again. runShell
	// returns only once the command's output AND the following prompt are through the pump,
	// so the tick below is guaranteed to be looking at a genuinely dirty session.
	runShell(t, eng, sid, "printf x")
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.True(t, bufExists(store.dir, sid),
		"after new output the next cadence tick must WriteBuf again (changed==true)")
	assert.Greater(t, countSavedForSession(store, sid), savesBefore,
		"after new output the next cadence tick must saveMeta again (changed==true)")

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestRegression_ResizeOnlyPersistsNewSize guards the §4.2/§8.4 resize-cache hole: a Resize
// with NO subsequent PTY output must still persist a fresh, larger-size blob — proving the
// resize marked the session dirty and invalidated the cached lastBlob, so the cadence flush
// does NOT hand back the stale pre-resize blob (which would restore at the wrong size and
// corrupt wrap). A session is born at the 80×24 default; after Resize(120×40) the next cadence
// tick must write a 120×40 CRWB1 header.
func TestRegression_ResizeOnlyPersistsNewSize(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid := newReadyShell(t, eng, "ws-resize", dir)

	// Flush the born-at-80×24 session to a clean dirty==false state with the 80×24 blob
	// cached. One flush suffices: the shell is at its prompt, so no further output can
	// re-dirty it. The committed header must read 80×24.
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	require.Equal(t, "CRWB1 80 24 0 10000", readBufHeader(t, store.dir, sid),
		"a freshly created session must persist an 80×24 CRWB1 header")

	// Resize to 120×40 with NO further write to the PTY, then delete the committed .buf so a
	// subsequent WriteBuf is unambiguously observable.
	require.NoError(t, eng.Resize(ctx, sid, 120, 40))
	require.NoError(t, os.Remove(filepath.Join(store.dir, sid+".buf")))

	// One cadence tick. Because the resize set dirty==true and nil'd lastBlob, Snapshot()
	// returns changed==true and re-serializes at the new size — the stale 80×24 cache is NOT
	// returned. Without the resize dirty-mark + cache-invalidation this recreates nothing (the
	// cache hands back the unchanged 80×24 blob and flushSessionOnce skips the write).
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	require.True(t, bufExists(store.dir, sid),
		"a resize-only tick must WriteBuf (changed==true from the resize dirty-mark)")
	assert.Equal(t, "CRWB1 120 40 0 10000", readBufHeader(t, store.dir, sid),
		"the resize-only cadence flush must persist a fresh 120×40 CRWB1 header, never the stale 80×24")

	require.NoError(t, eng.Kill(ctx, sid))
}
