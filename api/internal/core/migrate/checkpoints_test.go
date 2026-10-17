package migrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpoints_MarkIsDurableAndDoneSurvivesReopen(t *testing.T) {
	path := ledgerPathIn(t)
	step := ID{Number: 2, Name: "b"}
	l, err := openLedger(path)
	require.NoError(t, err)
	cp := newCheckpoints(l, "r1", step, fixedClock)
	assert.False(t, cp.Done("a"))

	require.NoError(t, cp.Mark("a", Counts{"files": 2}))

	assert.True(t, cp.Done("a"))
	onDisk, err := readLedger(path)
	require.NoError(t, err)
	assert.Equal(t, []record{{
		V: 1, Kind: "checkpoint", At: ledgerEpoch, Run: "r1",
		Step: 2, Name: "b", Key: "a", Counts: Counts{"files": 2},
	}}, onDisk.records, "the checkpoint is on disk before Mark returns")
	require.NoError(t, l.Close())

	reopened, err := openLedger(path)
	require.NoError(t, err)
	next := newCheckpoints(reopened, "r2", step, fixedClock)

	assert.True(t, next.Done("a"))
	assert.False(t, next.Done("b"))
	assert.Equal(t, Counts{"files": 2}, next.total())

	require.NoError(t, next.Mark("b", Counts{"files": 1, "bytes": 4}))
	assert.Equal(t, Counts{"files": 3, "bytes": 4}, next.total(), "total spans every attempt")
	assert.False(t, newCheckpoints(reopened, "r2", ID{Number: 3, Name: "c"}, fixedClock).Done("a"),
		"another step's checkpoints are not this step's")
	require.NoError(t, reopened.Close())
}

func TestCheckpoints_ARenamedOrRenumberedStepStartsFresh(t *testing.T) {
	l, err := openLedger(ledgerPathIn(t))
	require.NoError(t, err)
	first := newCheckpoints(l, "r1", ID{Number: 2, Name: "b"}, fixedClock)
	require.NoError(t, first.Mark("a", Counts{"files": 2}))

	assert.False(t, newCheckpoints(l, "r2", ID{Number: 2, Name: "renamed"}, fixedClock).Done("a"))
	assert.False(t, newCheckpoints(l, "r2", ID{Number: 3, Name: "b"}, fixedClock).Done("a"))
	assert.True(t, newCheckpoints(l, "r2", ID{Number: 2, Name: "b"}, fixedClock).Done("a"))
	require.NoError(t, l.Close())
}

func TestCheckpoints_MarkCopiesItsCounts(t *testing.T) {
	l, err := openLedger(ledgerPathIn(t))
	require.NoError(t, err)
	cp := newCheckpoints(l, "r1", ID{Number: 1, Name: "a"}, fixedClock)
	counts := Counts{"files": 1}

	require.NoError(t, cp.Mark("k", counts))
	counts["files"] = 99

	assert.Equal(t, Counts{"files": 1}, cp.total())
	assert.Equal(t, Counts{"files": 1}, l.view.records[0].Counts)
	require.NoError(t, l.Close())
}

func TestCheckpoints_FailedMarkLeavesTheKeyUndone(t *testing.T) {
	l, err := openLedger(ledgerPathIn(t))
	require.NoError(t, err)
	cp := newCheckpoints(l, "r1", ID{Number: 1, Name: "a"}, fixedClock)
	require.NoError(t, l.Close())

	require.Error(t, cp.Mark("k", Counts{"files": 1}))

	assert.False(t, cp.Done("k"), "a checkpoint that never reached disk must not read as done")
	assert.Empty(t, cp.total())
}
