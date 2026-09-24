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

// RelCodeActions rewrites the workspace edits inside a textDocument/codeAction
// result so every file they name is workspace-relative, the form the editor
// and the files API address files by. Commands and anything that does not
// decode are passed through unchanged.
func RelCodeActions(
	worktreePath string,
	raw json.RawMessage,
) json.RawMessage {
	var actions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &actions); err != nil {
		return raw
	}
	for _, action := range actions {
		edit, ok := action["edit"]
		if !ok {
			continue
		}
		action["edit"] = relRawWorkspaceEdit(worktreePath, edit)
	}
	out, err := json.Marshal(actions)
	if err != nil {
		return raw
	}
	return out
}

func relRawWorkspaceEdit(
	worktreePath string,
	raw json.RawMessage,
) json.RawMessage {
	var edit map[string]json.RawMessage
	if err := json.Unmarshal(raw, &edit); err != nil {
		return raw
	}
	if changes, ok := edit["changes"]; ok {
		var byURI map[string]json.RawMessage
		if err := json.Unmarshal(changes, &byURI); err == nil {
			byPath := make(map[string]json.RawMessage, len(byURI))
			for uri, edits := range byURI {
				byPath[WorkspaceRelPath(worktreePath, PathFromURI(uri))] = edits
			}
			if out, err := json.Marshal(byPath); err == nil {
				edit["changes"] = out
			}
		}
	}
	if docChanges, ok := edit["documentChanges"]; ok {
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(docChanges, &items); err == nil {
			for _, item := range items {
				var td map[string]json.RawMessage
				if err := json.Unmarshal(item["textDocument"], &td); err != nil {
					continue
				}
				var uri string
				if err := json.Unmarshal(td["uri"], &uri); err != nil {
					continue
				}
				rel, _ := json.Marshal(WorkspaceRelPath(worktreePath, PathFromURI(uri)))
				td["uri"] = rel
				item["textDocument"], _ = json.Marshal(td)
			}
			if out, err := json.Marshal(items); err == nil {
				edit["documentChanges"] = out
			}
		}
	}
	out, err := json.Marshal(edit)
	if err != nil {
		return raw
	}
	return out
}
