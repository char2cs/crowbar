package convert

import (
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
)

type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

type LSPDiagnostic struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source,omitempty"`
	Code     string   `json:"code,omitempty"`
}

type LSPLocation struct {
	URI   string   `json:"uri"`
	Range LSPRange `json:"range"`
}

type LSPTextEdit struct {
	Range   LSPRange `json:"range"`
	NewText string   `json:"newText"`
}

type LSPWorkspaceEdit struct {
	Changes map[string][]LSPTextEdit `json:"changes"`
}

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

func PositionFromLSP(
	line int,
	character int,
) lsp.Position {
	return lsp.Position{
		Line:      line,
		Character: character,
	}
}

func PathFromURI(
	uri string,
) string {
	return strings.TrimPrefix(uri, "file://")
}

func URIFromPath(
	path string,
) string {
	return "file://" + path
}

func WorkspaceRelPath(
	worktreePath string,
	path string,
) string {
	if worktreePath == "" || !filepath.IsAbs(path) {
		return path
	}

	rel, err := filepath.Rel(worktreePath, path)
	if err != nil {
		return path
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}

	return rel
}

func rangeFromLSP(
	r LSPRange,
) lsp.Range {
	return lsp.Range{
		Start: PositionFromLSP(r.Start.Line, r.Start.Character),
		End:   PositionFromLSP(r.End.Line, r.End.Character),
	}
}

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
