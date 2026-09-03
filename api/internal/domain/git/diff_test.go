package git_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaingit "github.com/char2cs/crowbar/api/internal/domain/git"
)

// TestFileDiff_MarshalJSON_NilSlicesBecomeEmptyArrays guards the wire contract
// described on FileDiff.MarshalJSON: a binary file leaves Lines nil and a diff
// with no hunks leaves Hunks nil, but clients always dereference both fields
// as arrays. Marshaling a zero-value FileDiff must never emit `null` for them.
func TestFileDiff_MarshalJSON_NilSlicesBecomeEmptyArrays(t *testing.T) {
	fd := domaingit.FileDiff{
		FilePath: "main.go",
		IsBinary: true,
	}

	out, err := json.Marshal(fd)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out, &decoded))

	assert.Equal(t, []any{}, decoded["lines"], "nil Lines must marshal to [] not null")
	assert.Equal(t, []any{}, decoded["hunks"], "nil Hunks must marshal to [] not null")
}

// TestFileDiff_MarshalJSON_PreservesPopulatedSlices ensures the nil-guard in
// MarshalJSON only substitutes empty slices for nil ones — it must not clobber
// or truncate a FileDiff that already carries real lines and hunks.
func TestFileDiff_MarshalJSON_PreservesPopulatedSlices(t *testing.T) {
	fd := domaingit.FileDiff{
		FilePath: "main.go",
		Lines: []domaingit.DiffLine{
			{LineType: domaingit.DiffLineAdded, Content: "+foo"},
		},
		Hunks: []domaingit.Hunk{
			{HunkID: "h1", Header: "@@ -1,1 +1,1 @@", StartLine: 0, EndLine: 0},
		},
	}

	out, err := json.Marshal(fd)
	require.NoError(t, err)

	var decoded domaingit.FileDiff
	require.NoError(t, json.Unmarshal(out, &decoded))

	require.Len(t, decoded.Lines, 1)
	assert.Equal(t, "+foo", decoded.Lines[0].Content)
	require.Len(t, decoded.Hunks, 1)
	assert.Equal(t, "h1", decoded.Hunks[0].HunkID)
}
