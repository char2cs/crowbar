package ledger_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestPreview_CutsABulkyBodyFieldAtALineAndNamesIt(t *testing.T) {
	diff := strings.Repeat("+a line of a diff\n", 1000)
	row, err := ledger.NewRow(ledger.KindToolCall, "T1", "", ledger.ToolCall{
		ToolID: "t1", Phase: ledger.ToolPost, Name: "edit", Diff: diff, Output: "ok",
	})
	require.NoError(t, err)
	row.Seq = 7

	got := ledger.Preview(row, 1024)

	assert.Equal(t, []string{"diff"}, got.Trimmed)
	body, err := ledger.DecodeBody[ledger.ToolCall](got)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(body.Diff), 1024)
	assert.True(t, strings.HasSuffix(body.Diff, "\n"), "cut on a whole line")
	assert.True(t, strings.HasPrefix(diff, body.Diff))
	assert.Equal(t, "ok", body.Output, "the fields it does not cut are as they were")
	assert.Equal(t, "edit", body.Name)
	assert.Equal(t, int64(7), got.Seq)
	assert.Less(t, len(got.Body), len(row.Body)/4)
}

func TestPreview_LeavesASmallRowAndTheLedgerRowAlone(t *testing.T) {
	row, err := ledger.NewRow(ledger.KindReasoning, "T1", "", ledger.Reasoning{Text: "short"})
	require.NoError(t, err)
	assert.Equal(t, row, ledger.Preview(row, 1024))

	big, err := ledger.NewRow(ledger.KindReasoning, "T1", "", ledger.Reasoning{Text: strings.Repeat("é", 2000)})
	require.NoError(t, err)
	before := string(big.Body)
	got := ledger.Preview(big, 1001)
	assert.Equal(t, before, string(big.Body), "a preview is a copy")
	body, err := ledger.DecodeBody[ledger.Reasoning](got)
	require.NoError(t, err)
	assert.Equal(t, strings.ToValidUTF8(body.Text, ""), body.Text, "never inside a rune")
	assert.Equal(t, []string{"text"}, got.Trimmed)
}

func TestPreview_NeverCutsWhatPaintsARow(t *testing.T) {
	row, err := ledger.NewRow(ledger.KindAssistantText, "T1", "", ledger.AssistantText{
		MessageID: "m", Index: ledger.IndexComplete, Text: strings.Repeat("word ", 5000),
	})
	require.NoError(t, err)
	assert.Equal(t, row, ledger.Preview(row, 1024))
}

func TestRegression_PreviewCutsABulkyToolOutput(t *testing.T) {
	row, err := ledger.NewRow(ledger.KindToolCall, "T1", "", ledger.ToolCall{
		ToolID: "t1", Phase: ledger.ToolPost, Output: strings.Repeat("out line\n", 1000),
	})
	require.NoError(t, err)

	got := ledger.Preview(row, 1024)

	assert.Equal(t, []string{"output"}, got.Trimmed)
	body, err := ledger.DecodeBody[ledger.ToolCall](got)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(body.Output), 1024)
}
