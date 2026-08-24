package tree

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A dirty id the snapshot does not hold is SKIPPED rather than dereferenced.
// Nothing in the usecase can currently produce one — the plan is built from the
// snapshot's own rows and a dropped row leaves the dirty set with it — but the
// two are separate structures, and the cost of them disagreeing is a nil
// dereference inside a write loop that has already persisted half a move. The
// guard is cheap; discovering it was missing would not be.
//
// It is exercised from inside the package because there is no way to ask for
// this from outside: the seam only exists between the plan and the rows.
func TestWriteRow_SkipsAnIDTheSnapshotDoesNotHold(t *testing.T) {
	u := &chatFolderUsecase{}
	snapshot := newTreeSnapshot(nil, nil)

	row, err := u.writeRow(context.Background(), snapshot, "ghost")

	require.NoError(t, err)
	assert.Nil(t, row)
}

// drop is called for every folder a cascade removes, and a cascade can be handed
// the same id twice by two levels of the walk. The second call must be a no-op,
// not a rebuild of the index map over a row that is already gone.
func TestDrop_IgnoresAnIDTheSnapshotDoesNotHold(t *testing.T) {
	snapshot := newTreeSnapshot(nil, nil)

	snapshot.drop("ghost")

	assert.Empty(t, snapshot.folders)
}

// dropChat carries the same guard for the same reason, on the kind whose removal
// is driven by a subtree walk rather than by a single id.
func TestDropChat_IgnoresAnIDTheSnapshotDoesNotHold(t *testing.T) {
	snapshot := newTreeSnapshot(nil, nil)

	snapshot.dropChat("ghost")

	assert.Empty(t, snapshot.chats)
}
