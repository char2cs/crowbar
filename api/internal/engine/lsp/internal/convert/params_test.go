package convert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
)

func TestTextDocumentPositionParams(t *testing.T) {
	p := convert.TextDocumentPositionParams("/p/main.go", lsp.Position{Line: 3, Character: 7})

	td := p["textDocument"].(map[string]any)
	assert.Equal(t, "file:///p/main.go", td["uri"])

	pos := p["position"].(map[string]any)
	assert.Equal(t, 3, pos["line"])
	assert.Equal(t, 7, pos["character"])
}

func TestReferenceParams_IncludesDeclaration(t *testing.T) {
	p := convert.ReferenceParams("/p/main.go", lsp.Position{Line: 1, Character: 2})

	ctx := p["context"].(map[string]any)
	assert.Equal(t, true, ctx["includeDeclaration"])
}

func TestRenameParams(t *testing.T) {
	p := convert.RenameParams("/p/main.go", lsp.Position{Line: 1, Character: 2}, "Renamed")
	assert.Equal(t, "Renamed", p["newName"])

	td := p["textDocument"].(map[string]any)
	assert.Equal(t, "file:///p/main.go", td["uri"])
}

func TestCodeActionParams(t *testing.T) {
	rng := lsp.Range{
		Start: lsp.Position{Line: 1, Character: 0},
		End:   lsp.Position{Line: 2, Character: 5},
	}
	p := convert.CodeActionParams("/p/main.go", rng)

	r := p["range"].(map[string]any)
	start := r["start"].(map[string]any)
	end := r["end"].(map[string]any)
	assert.Equal(t, 1, start["line"])
	assert.Equal(t, 5, end["character"])

	ctx := p["context"].(map[string]any)
	assert.Contains(t, ctx, "diagnostics")
}

func TestDocumentSymbolParams(t *testing.T) {
	p := convert.DocumentSymbolParams("/p/main.go")
	td := p["textDocument"].(map[string]any)
	assert.Equal(t, "file:///p/main.go", td["uri"])
}
