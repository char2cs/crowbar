package convert

import (
	"encoding/json"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TextDocumentPositionParams(
	path string,
	pos lsp.Position,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"position":     position(pos),
	}
}

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

func CodeActionParams(
	path string,
	rng lsp.Range,
	diagnostics json.RawMessage,
) map[string]any {
	var diags any = []any{}
	if len(diagnostics) > 0 && string(diagnostics) != "null" {
		diags = diagnostics
	}
	return map[string]any{
		"textDocument": textDocument(path),
		"range":        rangeObject(rng),
		"context":      map[string]any{"diagnostics": diags},
	}
}

func FormattingParams(
	path string,
	options lsp.FormattingOptions,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"options": map[string]any{
			"tabSize":      options.TabSize,
			"insertSpaces": options.InsertSpaces,
		},
	}
}

func DocumentSymbolParams(
	path string,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
	}
}

func SemanticTokensDeltaParams(
	path string,
	previousResultID string,
) map[string]any {
	return map[string]any{
		"textDocument":     textDocument(path),
		"previousResultId": previousResultID,
	}
}

func SemanticTokensRangeParams(
	path string,
	rng lsp.Range,
) map[string]any {
	return map[string]any{
		"textDocument": textDocument(path),
		"range":        rangeObject(rng),
	}
}

func ExecuteCommandParams(
	command string,
	arguments json.RawMessage,
) map[string]any {
	params := map[string]any{"command": command}
	if len(arguments) > 0 && string(arguments) != "null" {
		params["arguments"] = arguments
	}
	return params
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
