package convert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
)

// --- SeverityFromLSP ---

func TestSeverityFromLSP_Error(t *testing.T) {
	assert.Equal(t, "error", convert.SeverityFromLSP(1))
}

func TestSeverityFromLSP_Warning(t *testing.T) {
	assert.Equal(t, "warning", convert.SeverityFromLSP(2))
}

func TestSeverityFromLSP_Info(t *testing.T) {
	assert.Equal(t, "info", convert.SeverityFromLSP(3))
}

func TestSeverityFromLSP_Hint(t *testing.T) {
	assert.Equal(t, "hint", convert.SeverityFromLSP(4))
}

func TestSeverityFromLSP_Unknown(t *testing.T) {
	assert.Equal(t, "error", convert.SeverityFromLSP(0))
	assert.Equal(t, "error", convert.SeverityFromLSP(99))
	assert.Equal(t, "error", convert.SeverityFromLSP(-1))
}

// --- PositionFromLSP ---

func TestPositionFromLSP_ZeroBased(t *testing.T) {
	pos := convert.PositionFromLSP(5, 12)
	assert.Equal(t, lsp.Position{Line: 5, Character: 12}, pos)
}

func TestPositionFromLSP_Zero(t *testing.T) {
	pos := convert.PositionFromLSP(0, 0)
	assert.Equal(t, lsp.Position{Line: 0, Character: 0}, pos)
}

// --- URI helpers ---

func TestPathFromURI_StripFileScheme(t *testing.T) {
	path := convert.PathFromURI("file:///home/user/project/main.go")
	assert.Equal(t, "/home/user/project/main.go", path)
}

func TestPathFromURI_NoFileScheme(t *testing.T) {
	path := convert.PathFromURI("/home/user/project/main.go")
	assert.Equal(t, "/home/user/project/main.go", path)
}

func TestURIFromPath_AddsFileScheme(t *testing.T) {
	uri := convert.URIFromPath("/home/user/project/main.go")
	assert.Equal(t, "file:///home/user/project/main.go", uri)
}

func TestURIRoundTrip(t *testing.T) {
	original := "/home/user/project/main.go"
	uri := convert.URIFromPath(original)
	restored := convert.PathFromURI(uri)
	assert.Equal(t, original, restored)
}

// --- DiagnosticFromLSP ---

func TestDiagnosticFromLSP_Full(t *testing.T) {
	d := convert.DiagnosticFromLSP(
		"file:///project/main.go",
		convert.LSPDiagnostic{
			Range: convert.LSPRange{
				Start: convert.LSPPosition{Line: 2, Character: 4},
				End:   convert.LSPPosition{Line: 2, Character: 10},
			},
			Severity: 1,
			Message:  "undefined: foo",
			Source:   "gopls",
			Code:     "E001",
		},
	)
	assert.Equal(t, "/project/main.go", d.FilePath)
	assert.Equal(t, "error", d.Severity)
	assert.Equal(t, "undefined: foo", d.Message)
	assert.Equal(t, "gopls", d.Source)
	assert.Equal(t, "E001", d.Code)
	assert.Equal(t, lsp.Position{Line: 2, Character: 4}, d.Range.Start)
	assert.Equal(t, lsp.Position{Line: 2, Character: 10}, d.Range.End)
}

func TestDiagnosticFromLSP_OptionalFields(t *testing.T) {
	d := convert.DiagnosticFromLSP(
		"file:///project/main.go",
		convert.LSPDiagnostic{
			Range: convert.LSPRange{
				Start: convert.LSPPosition{Line: 0, Character: 0},
				End:   convert.LSPPosition{Line: 0, Character: 0},
			},
			Severity: 2,
			Message:  "warning message",
		},
	)
	assert.Equal(t, "warning", d.Severity)
	assert.Equal(t, "", d.Source)
	assert.Equal(t, "", d.Code)
}

// --- LocationsFromLSP ---

func TestLocationsFromLSP_Multiple(t *testing.T) {
	raw := []convert.LSPLocation{
		{
			URI: "file:///project/a.go",
			Range: convert.LSPRange{
				Start: convert.LSPPosition{Line: 1, Character: 0},
				End:   convert.LSPPosition{Line: 1, Character: 5},
			},
		},
		{
			URI: "file:///project/b.go",
			Range: convert.LSPRange{
				Start: convert.LSPPosition{Line: 10, Character: 2},
				End:   convert.LSPPosition{Line: 10, Character: 8},
			},
		},
	}
	locs := convert.LocationsFromLSP(raw)
	require.Len(t, locs, 2)
	assert.Equal(t, "/project/a.go", locs[0].FilePath)
	assert.Equal(t, lsp.Position{Line: 1, Character: 0}, locs[0].Range.Start)
	assert.Equal(t, "/project/b.go", locs[1].FilePath)
	assert.Equal(t, lsp.Position{Line: 10, Character: 2}, locs[1].Range.Start)
}

func TestLocationsFromLSP_Empty(t *testing.T) {
	locs := convert.LocationsFromLSP(nil)
	assert.Empty(t, locs)
}

// --- WorkspaceEditFromLSP ---

func TestWorkspaceEditFromLSP_Changes(t *testing.T) {
	raw := convert.LSPWorkspaceEdit{
		Changes: map[string][]convert.LSPTextEdit{
			"file:///project/main.go": {
				{
					Range: convert.LSPRange{
						Start: convert.LSPPosition{Line: 0, Character: 0},
						End:   convert.LSPPosition{Line: 0, Character: 3},
					},
					NewText: "pkg",
				},
			},
		},
	}
	we := convert.WorkspaceEditFromLSP(raw)
	edits, ok := we.Changes["/project/main.go"]
	require.True(t, ok)
	require.Len(t, edits, 1)
	assert.Equal(t, "pkg", edits[0].NewText)
	assert.Equal(t, lsp.Position{Line: 0, Character: 0}, edits[0].Range.Start)
}

func TestWorkspaceEditFromLSP_EmptyChanges(t *testing.T) {
	we := convert.WorkspaceEditFromLSP(convert.LSPWorkspaceEdit{})
	assert.Empty(t, we.Changes)
}

func TestWorkspaceEditFromLSP_NilChanges(t *testing.T) {
	raw := convert.LSPWorkspaceEdit{Changes: nil}
	we := convert.WorkspaceEditFromLSP(raw)
	assert.Empty(t, we.Changes)
}
