package terminal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// TestStats_CountsSuspendedPlaceholders verifies that suspended placeholders are
// counted in both the suspended tally and the total model-byte estimate, using
// their lazily-sized placeholder blobs (not a phantom 256 KB each). Previously placeholders
// were uncounted, letting them grow without bound.
func TestStats_CountsSuspendedPlaceholders(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()

	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	scrollbacks := map[string][]byte{
		"ph-1": []byte("aaaa"),
		"ph-2": []byte("bbbbbbbb"),
		"ph-3": []byte("cc"),
	}
	var wantBytes int64
	for id, sb := range scrollbacks {
		require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
			SessionID:   id,
			WorkspaceID: "ws-1",
			Shell:       "/bin/sh",
			State:       "suspended",
		}, sb))
		wantBytes += int64(len(sb))
	}

	active, detached, suspended, modelBytes, _, _ := eng.Stats()
	assert.Zero(t, active+detached, "no live sessions")
	assert.Equal(t, 3, suspended, "all three placeholders counted as suspended")
	assert.Equal(t, wantBytes, modelBytes,
		"model bytes must equal the sum of stored blob lengths, not 3×256 KB")
}

// TestMaintenance_EvictsOldestPlaceholdersOverCeiling verifies that when the
// global session-count ceiling counts placeholders and is exceeded, the
// maintenance sweep evicts the least-recently-active placeholder(s): registry
// entry removed, .buf deleted, meta row deleted.
func TestMaintenance_EvictsOldestPlaceholdersOverCeiling(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()

	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	restoreN := terminal.SetMaxTotalSessionsForTest(2)
	defer restoreN()
	restoreB := terminal.SetMaxTotalModelBytesForTest(1 << 30) // keep byte ceiling out of the way
	defer restoreB()

	ids := []string{"ph-old", "ph-mid", "ph-new"}
	base := time.Now()
	for i, id := range ids {
		sb := []byte("scroll-" + id)
		require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
			SessionID:   id,
			WorkspaceID: "ws-1",
			Shell:       "/bin/sh",
			State:       "suspended",
		}, sb))
		require.NoError(t, os.WriteFile(filepath.Join(store.dir, id+".buf"), sb, 0o644))
		terminal.SetLastActiveForTest(eng, id, base.Add(time.Duration(i)*time.Minute))
	}

	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// Cap of 2 with 3 placeholders → the single oldest (ph-old) is evicted.
	assert.False(t, eng.SessionExists(ctx, "ph-old"), "oldest placeholder must be evicted")
	assert.True(t, eng.SessionExists(ctx, "ph-mid"), "newer placeholders survive")
	assert.True(t, eng.SessionExists(ctx, "ph-new"), "newer placeholders survive")
	assert.True(t, store.hasDeleted("ph-old"), "evicted placeholder meta row must be deleted")
	assert.False(t, bufExists(store.dir, "ph-old"), "evicted placeholder .buf must be deleted")
}
