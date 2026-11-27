package git_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaingit "github.com/char2cs/crowbar/api/internal/domain/git"
)

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
