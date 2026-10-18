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
