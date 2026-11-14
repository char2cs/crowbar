package ledger_test

import (
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

var updateKindsFixture = flag.Bool("update-kinds-fixture", false, "write the row and marker kinds the web tests check")

// kindsFixture is what the web's ledger types are checked against: written
// only from here, so it never drifts from what Check accepts.
const kindsFixture = "../../../../app/panes/src/__tests__/__fixtures__/ledger-kinds.json"

func TestCheck_TheKindsTheWebChecksAreTheOnesItAccepts(t *testing.T) {
	got, err := json.MarshalIndent(map[string]any{"rowKinds": ledger.RowKinds(), "markerKinds": ledger.MarkerKinds()}, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	if *updateKindsFixture {
		require.NoError(t, os.WriteFile(kindsFixture, got, 0o600))
		return
	}
	want, err := os.ReadFile(kindsFixture)
	require.NoError(t, err, "write it with -args -update-kinds-fixture")
	assert.Equal(t, string(want), string(got), "the kinds drifted: rerun with -args -update-kinds-fixture")
}

// Every Marker* constant is a kind Check accepts: a new one missing from the
// accepted list fails here, not on the first row a CLI sends of it.
func TestCheck_EveryMarkerConstantIsAKindItAccepts(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "marker.go", nil, 0)
	require.NoError(t, err)
	var declared []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if strings.HasPrefix(name.Name, "Marker") && ok && lit.Kind == token.STRING {
				v, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				declared = append(declared, v)
			}
		}
		return true
	})
	slices.Sort(declared)

	require.NotEmpty(t, declared)
	assert.Equal(t, declared, ledger.MarkerKinds())
}

func TestCheck_AMarkerNeedsAKnownKind(t *testing.T) {
	changed, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerSessionChanged, Detail: "S2"})
	require.NoError(t, err)
	blank, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Detail: "S2"})
	require.NoError(t, err)
	unknown, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: "weather"})
	require.NoError(t, err)

	require.NoError(t, ledger.Check(ledger.State{}, changed))
	require.ErrorIs(t, ledger.Check(ledger.State{}, blank), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, unknown), ledger.ErrMalformedRow)

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
