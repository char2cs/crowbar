package convert_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
)

func TestLocationsFromResult_Array(t *testing.T) {
	raw := json.RawMessage(`[
		{"uri":"file:///p/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}},
		{"uri":"file:///p/b.go","range":{"start":{"line":2,"character":1},"end":{"line":2,"character":5}}}
	]`)
	locs, err := convert.LocationsFromResult(raw)
	require.NoError(t, err)
	require.Len(t, locs, 2)
	assert.Equal(t, "/p/a.go", locs[0].FilePath)
	assert.Equal(t, "/p/b.go", locs[1].FilePath)
}

func TestLocationsFromResult_SingleObject(t *testing.T) {
	raw := json.RawMessage(`{"uri":"file:///p/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}`)
	locs, err := convert.LocationsFromResult(raw)
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, "/p/a.go", locs[0].FilePath)
	assert.Equal(t, 1, locs[0].Range.Start.Line)
}

func TestLocationsFromResult_Null(t *testing.T) {
	locs, err := convert.LocationsFromResult(json.RawMessage(`null`))
	require.NoError(t, err)
	assert.Empty(t, locs)
}

func TestLocationsFromResult_Empty(t *testing.T) {
	locs, err := convert.LocationsFromResult(nil)
	require.NoError(t, err)
	assert.Empty(t, locs)
}

func TestLocationsFromResult_BadArray(t *testing.T) {
	_, err := convert.LocationsFromResult(json.RawMessage(`[123]`))
	require.Error(t, err)
}

func TestLocationsFromResult_BadObject(t *testing.T) {
	_, err := convert.LocationsFromResult(json.RawMessage(`{"uri":123}`))
	require.Error(t, err)
}

func TestWorkspaceEditFromResult_Changes(t *testing.T) {
	raw := json.RawMessage(`{"changes":{"file:///p/a.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":3}},"newText":"pkg"}]}}`)
	we, err := convert.WorkspaceEditFromResult(raw)
	require.NoError(t, err)
	edits, ok := we.Changes["/p/a.go"]
	require.True(t, ok)
	require.Len(t, edits, 1)
	assert.Equal(t, "pkg", edits[0].NewText)
}

func TestWorkspaceEditFromResult_Null(t *testing.T) {
	we, err := convert.WorkspaceEditFromResult(json.RawMessage(`null`))
	require.NoError(t, err)
	assert.Empty(t, we.Changes)
}

func TestWorkspaceEditFromResult_Bad(t *testing.T) {
	_, err := convert.WorkspaceEditFromResult(json.RawMessage(`{"changes":123}`))
	require.Error(t, err)
}
