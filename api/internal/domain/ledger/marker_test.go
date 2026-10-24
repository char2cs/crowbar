package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestCheck_AMarkerNeedsAKind(t *testing.T) {
	changed, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerSessionChanged, Detail: "S2"})
	require.NoError(t, err)
	blank, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Detail: "S2"})
	require.NoError(t, err)

	require.NoError(t, ledger.Check(ledger.State{}, changed))
	require.ErrorIs(t, ledger.Check(ledger.State{}, blank), ledger.ErrMalformedRow)

	st := ledger.Fold("c1", sequenced(t, changed))
	assert.Empty(t, st.Conversations(), "a marker is shown, not folded")
	assert.Equal(t, int64(1), st.Head())
}

func compaction(t *testing.T, kind, providerTurnID string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindMarker, "", ledger.Marker{Kind: kind, ProviderTurnID: providerTurnID})
}

func TestState_ACompactionIsOpenUntilItEnds(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t, compaction(t, ledger.MarkerCompactionStarted, "pt-1")))
	open, turn := st.Compaction()
	assert.True(t, open)
	assert.Equal(t, "pt-1", turn)

	st = ledger.Fold("c1", sequenced(t,
		compaction(t, ledger.MarkerCompactionStarted, "pt-1"), compaction(t, ledger.MarkerCompactionEnded, ""),
	))
	open, turn = st.Compaction()
	assert.False(t, open)
	assert.Equal(t, "pt-1", turn, "only its own turn's end disarms it")

	st = ledger.Fold("c1", sequenced(t,
		compaction(t, ledger.MarkerCompactionStarted, "pt-1"), compaction(t, ledger.MarkerCompactionEnded, "pt-1"),
	))
	_, turn = st.Compaction()
	assert.Empty(t, turn)
}
