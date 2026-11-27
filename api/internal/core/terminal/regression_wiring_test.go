package terminal_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

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

func TestRegression_IdleSessionNoCadenceWrite(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid := newReadyShell(t, eng, "chat-idle", dir)

	terminal.RunMaintenanceOnceForTest(eng, ctx)
	require.True(t, bufExists(store.dir, sid), "first cadence flush must write the .buf")

	require.NoError(t, os.Remove(filepath.Join(store.dir, sid+".buf")))
	savesBefore := countSavedForSession(store, sid)

	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.False(t, bufExists(store.dir, sid),
		"an idle (unchanged) cadence tick must perform NO WriteBuf — the deleted .buf must stay gone")
	assert.Equal(t, savesBefore, countSavedForSession(store, sid),
		"an idle (unchanged) cadence tick must perform NO saveMeta")

	runShell(t, eng, sid, "printf x")
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.True(t, bufExists(store.dir, sid),
		"after new output the next cadence tick must WriteBuf again (changed==true)")
	assert.Greater(t, countSavedForSession(store, sid), savesBefore,
		"after new output the next cadence tick must saveMeta again (changed==true)")

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestRegression_ResizeOnlyPersistsNewSize(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid := newReadyShell(t, eng, "chat-resize", dir)

	terminal.RunMaintenanceOnceForTest(eng, ctx)
	require.Equal(t, "CRWB1 80 24 0 10000", readBufHeader(t, store.dir, sid),
		"a freshly created session must persist an 80×24 CRWB1 header")

	require.NoError(t, eng.Resize(ctx, sid, 120, 40))
	require.NoError(t, os.Remove(filepath.Join(store.dir, sid+".buf")))

	terminal.RunMaintenanceOnceForTest(eng, ctx)

	require.True(t, bufExists(store.dir, sid),
		"a resize-only tick must WriteBuf (changed==true from the resize dirty-mark)")
	assert.Equal(t, "CRWB1 120 40 0 10000", readBufHeader(t, store.dir, sid),
		"the resize-only cadence flush must persist a fresh 120×40 CRWB1 header, never the stale 80×24")

	require.NoError(t, eng.Kill(ctx, sid))
}
