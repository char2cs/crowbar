package convert

import (
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// LocationsFromResult decodes a textDocument/definition or references result
// into Crowbar Locations. The LSP result is either a single Location object or
// an array of Locations; a null/empty result yields an empty slice.
func LocationsFromResult(
	raw json.RawMessage,
) ([]lsp.Location, error) {
	if isNull(raw) {
		return nil, nil
	}
	if raw[0] == '[' {
		return locationsFromArray(raw)
	}
	return locationFromObject(raw)
}

func locationsFromArray(
	raw json.RawMessage,
) ([]lsp.Location, error) {
	var arr []LSPLocation
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("convert: locations array: %w", err)
	}
	return LocationsFromLSP(arr), nil
}

func locationFromObject(
	raw json.RawMessage,
) ([]lsp.Location, error) {
	var one LSPLocation
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, fmt.Errorf("convert: location object: %w", err)
	}
	return LocationsFromLSP([]LSPLocation{one}), nil
}

// WorkspaceEditFromResult decodes a textDocument/rename result into a Crowbar
// WorkspaceEdit. A null result yields an empty edit.
func WorkspaceEditFromResult(
	raw json.RawMessage,
) (lsp.WorkspaceEdit, error) {
	if isNull(raw) {
		return lsp.WorkspaceEdit{}, nil
	}
	var we LSPWorkspaceEdit
	if err := json.Unmarshal(raw, &we); err != nil {
		return lsp.WorkspaceEdit{}, fmt.Errorf("convert: workspace edit: %w", err)
	}
	return WorkspaceEditFromLSP(we), nil
}

func isNull(
	raw json.RawMessage,
) bool {
	return len(raw) == 0 || string(raw) == "null"
}
