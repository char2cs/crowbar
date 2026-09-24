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
		action["edit"] = RelWorkspaceEdit(worktreePath, edit)
	}
	out, err := json.Marshal(actions)
	if err != nil {
		return raw
	}
	return out
}

// RelWorkspaceEdit rewrites every file a raw LSP WorkspaceEdit names
// (changes keys, documentChanges URIs) to its workspace-relative path.
// Anything that does not decode is passed through unchanged.
func RelWorkspaceEdit(
	worktreePath string,
	raw json.RawMessage,
) json.RawMessage {
	var edit map[string]json.RawMessage
	if err := json.Unmarshal(raw, &edit); err != nil {
		return raw
	}
	if changes, ok := edit["changes"]; ok {
		edit["changes"] = relChanges(worktreePath, changes)
	}
	if docChanges, ok := edit["documentChanges"]; ok {
		edit["documentChanges"] = relDocumentChanges(worktreePath, docChanges)
	}
	out, err := json.Marshal(edit)
	if err != nil {
		return raw
	}
	return out
}

// relChanges rewrites the keys of a WorkspaceEdit's changes map (file URIs).
func relChanges(
	worktreePath string,
	raw json.RawMessage,
) json.RawMessage {
	var byURI map[string]json.RawMessage
	if err := json.Unmarshal(raw, &byURI); err != nil {
		return raw
	}
	byPath := make(map[string]json.RawMessage, len(byURI))
	for uri, edits := range byURI {
		byPath[WorkspaceRelPath(worktreePath, PathFromURI(uri))] = edits
	}
	out, err := json.Marshal(byPath)
	if err != nil {
		return raw
	}
	return out
}

// relDocumentChanges rewrites each documentChanges entry's textDocument URI.
func relDocumentChanges(
	worktreePath string,
	raw json.RawMessage,
) json.RawMessage {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return raw
	}
	for _, item := range items {
		relTextDocument(worktreePath, item)
	}
	out, err := json.Marshal(items)
	if err != nil {
		return raw
	}
	return out
}

func relTextDocument(
	worktreePath string,
	item map[string]json.RawMessage,
) {
	var td map[string]json.RawMessage
	if err := json.Unmarshal(item["textDocument"], &td); err != nil {
		return
	}
	var uri string
	if err := json.Unmarshal(td["uri"], &uri); err != nil {
		return
	}
	rel, _ := json.Marshal(WorkspaceRelPath(worktreePath, PathFromURI(uri)))
	td["uri"] = rel
	item["textDocument"], _ = json.Marshal(td)
}
