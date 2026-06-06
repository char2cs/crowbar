// Package convert translates between LSP wire types and the Crowbar domain
// DTOs defined in internal/domain/lsp (10 §6). Each mapping is a small,
// pure function with no side effects.
package convert

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// LSP wire types — defined here so the convert package owns its own
// coupling surface and callers never import raw LSP structs.

// LSPPosition is the LSP wire Position (0-based line and character).
type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// LSPRange is the LSP wire Range.
type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

// LSPDiagnostic is the LSP wire Diagnostic.
type LSPDiagnostic struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source,omitempty"`
	Code     string   `json:"code,omitempty"`
}

// LSPLocation is the LSP wire Location.
type LSPLocation struct {
	URI   string   `json:"uri"`
	Range LSPRange `json:"range"`
}

// LSPTextEdit is the LSP wire TextEdit.
type LSPTextEdit struct {
	Range   LSPRange `json:"range"`
	NewText string   `json:"newText"`
}

// LSPWorkspaceEdit is the LSP wire WorkspaceEdit.
type LSPWorkspaceEdit struct {
	Changes map[string][]LSPTextEdit `json:"changes"`
}

// SeverityFromLSP converts an LSP DiagnosticSeverity integer to the Crowbar
// string label. Unknown values default to "error".
func SeverityFromLSP(
	severity int,
) string {
	switch severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "error"
	}
}

// PositionFromLSP converts LSP 0-based line/character integers to a
// domain Position (already 0-based; mapping is 1:1).
func PositionFromLSP(
	line int,
	character int,
) lsp.Position {
	return lsp.Position{
		Line:      line,
		Character: character,
	}
}

// PathFromURI strips the "file://" scheme from a URI, returning a plain
// filesystem path. If the URI has no "file://" prefix it is returned as-is.
func PathFromURI(
	uri string,
) string {
	return strings.TrimPrefix(uri, "file://")
}

// URIFromPath converts a filesystem path to a file URI by prepending
// "file://".
func URIFromPath(
	path string,
) string {
	return "file://" + path
}

// rangeFromLSP converts an LSP wire Range to a domain Range.
func rangeFromLSP(
	r LSPRange,
) lsp.Range {
	return lsp.Range{
		Start: PositionFromLSP(r.Start.Line, r.Start.Character),
		End:   PositionFromLSP(r.End.Line, r.End.Character),
	}
}

// DiagnosticFromLSP converts a single LSP wire Diagnostic to a Crowbar
// Diagnostic, filling FilePath from the document URI.
func DiagnosticFromLSP(
	uri string,
	raw LSPDiagnostic,
) lsp.Diagnostic {
	return lsp.Diagnostic{
		FilePath: PathFromURI(uri),
		Range:    rangeFromLSP(raw.Range),
		Severity: SeverityFromLSP(raw.Severity),
		Message:  raw.Message,
		Source:   raw.Source,
		Code:     raw.Code,
	}
}

// LocationsFromLSP converts a slice of LSP wire Locations to Crowbar
// Locations.
func LocationsFromLSP(
	raw []LSPLocation,
) []lsp.Location {
	if len(raw) == 0 {
		return nil
	}
	out := make([]lsp.Location, len(raw))
	for i, loc := range raw {
		out[i] = lsp.Location{
			FilePath: PathFromURI(loc.URI),
			Range:    rangeFromLSP(loc.Range),
		}
	}
	return out
}

// WorkspaceEditFromLSP converts an LSP wire WorkspaceEdit (URI-keyed) to a
// Crowbar WorkspaceEdit (path-keyed).
func WorkspaceEditFromLSP(
	raw LSPWorkspaceEdit,
) lsp.WorkspaceEdit {
	if len(raw.Changes) == 0 {
		return lsp.WorkspaceEdit{}
	}
	changes := make(map[string][]lsp.TextEdit, len(raw.Changes))
	for uri, edits := range raw.Changes {
		changes[PathFromURI(uri)] = textEditsFromLSP(edits)
	}
	return lsp.WorkspaceEdit{Changes: changes}
}

// textEditsFromLSP converts a slice of LSP wire TextEdits to Crowbar
// TextEdits.
func textEditsFromLSP(
	raw []LSPTextEdit,
) []lsp.TextEdit {
	out := make([]lsp.TextEdit, len(raw))
	for i, e := range raw {
		out[i] = lsp.TextEdit{
			Range:   rangeFromLSP(e.Range),
			NewText: e.NewText,
		}
	}
	return out
}
