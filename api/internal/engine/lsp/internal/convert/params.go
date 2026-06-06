package convert

import "github.com/char2cs/crowbar/api/internal/domain/lsp"

// TextDocumentPositionParams builds the LSP textDocument/position parameter
// object shared by completion, hover, definition, and references requests. The
// file path is converted to a file URI and the position is passed through 1:1.
func TextDocumentPositionParams(
	path string,
	pos lsp.Position,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"position":     position(pos),
	}
}

// ReferenceParams builds the textDocument/references parameter object, including
// the declaration so the result set covers the symbol's definition.
func ReferenceParams(
	path string,
	pos lsp.Position,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"position":     position(pos),
		"context":      map[string]any{"includeDeclaration": true},
	}
}

// RenameParams builds the textDocument/rename parameter object.
func RenameParams(
	path string,
	pos lsp.Position,
	newName string,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"position":     position(pos),
		"newName":      newName,
	}
}

// CodeActionParams builds the textDocument/codeAction parameter object scoped to
// the given range.
func CodeActionParams(
	path string,
	rng lsp.Range,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"range":        rangeObject(rng),
		"context":      map[string]any{"diagnostics": []any{}},
	}
}

// DocumentSymbolParams builds the textDocument/documentSymbol parameter object.
func DocumentSymbolParams(
	path string,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
	}
}

func textDocument(
	path string,
) map[string]any {
	return map[string]any{"uri": URIFromPath(path)}
}

func position(
	pos lsp.Position,
) map[string]any {
	return map[string]any{
		"line":      pos.Line,
		"character": pos.Character,
	}
}

func rangeObject(
	rng lsp.Range,
) map[string]any {
	return map[string]any{
		"start": position(rng.Start),
		"end":   position(rng.End),
	}
}
