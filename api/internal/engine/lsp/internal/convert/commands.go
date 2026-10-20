package convert

import (
	"encoding/json"
	"fmt"
)

// ClientCodeActions shapes a textDocument/codeAction result for the editor:
// every workspace edit names workspace-relative files, and only commands the
// server itself runs (canRun) survive — a bare command it cannot run is
// dropped, and an action's unrunnable command is stripped, keeping its edit.
// A result that does not decode is passed through unchanged; a command that
// does not decode is an error.
func ClientCodeActions(
	worktreePath string,
	raw json.RawMessage,
	canRun func(string) bool,
) (json.RawMessage, error) {
	var actions []map[string]json.RawMessage
	if !decodes(raw, &actions) {
		return raw, nil
	}
	kept := make([]map[string]json.RawMessage, 0, len(actions))
	for _, action := range actions {
		keep, err := clientCodeAction(worktreePath, action, canRun)
		if err != nil {
			return nil, err
		}
		if keep {
			kept = append(kept, action)
		}
	}
	return marshalOr(kept, raw), nil
}

// clientCodeAction rewrites one Command | CodeAction in place and reports
// whether the editor can still do anything with it.
func clientCodeAction(
	worktreePath string,
	action map[string]json.RawMessage,
	canRun func(string) bool,
) (bool, error) {
	var bare string
	if json.Unmarshal(action["command"], &bare) == nil && bare != "" {
		return canRun(bare), nil
	}
	if err := dropUnrunnable(action, canRun); err != nil {
		return false, err
	}
	if edit, ok := action["edit"]; ok {
		action["edit"] = RelWorkspaceEdit(worktreePath, edit)
	}
	_, hasEdit := action["edit"]
	_, hasCommand := action["command"]
	return hasEdit || hasCommand, nil
}

// dropUnrunnable strips an action's command when the server cannot run it.
func dropUnrunnable(
	action map[string]json.RawMessage,
	canRun func(string) bool,
) error {
	cmd, ok := action["command"]
	if !ok {
		return nil
	}
	name, err := commandName(cmd)
	if err != nil || canRun(name) {
		return err
	}
	delete(action, "command")
	return nil
}

// ClientLenses shapes a textDocument/codeLens result (an array) or a
// codeLens/resolve result (one lens) for the editor: a lens whose command the
// server cannot run keeps its title but loses the command, so the editor
// shows it as a label rather than a button that fails. A result that does not
// decode is passed through unchanged; a command that does not decode is an error.
func ClientLenses(
	raw json.RawMessage,
	canRun func(string) bool,
) (json.RawMessage, error) {
	var lenses []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &lenses); err == nil {
		for _, lens := range lenses {
			if err := clientLens(lens, canRun); err != nil {
				return nil, err
			}
		}
		return marshalOr(lenses, raw), nil
	}
	var lens map[string]json.RawMessage
	if !decodes(raw, &lens) || lens == nil {
		return raw, nil
	}
	if err := clientLens(lens, canRun); err != nil {
		return nil, err
	}
	return marshalOr(lens, raw), nil
}

func clientLens(
	lens map[string]json.RawMessage,
	canRun func(string) bool,
) error {
	cmd, ok := lens["command"]
	if !ok {
		return nil
	}
	name, err := commandName(cmd)
	if err != nil || canRun(name) {
		return err
	}
	var title struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(cmd, &title); err != nil {
		return fmt.Errorf("convert: lens command title: %w", err)
	}
	lens["command"] = marshalOr(map[string]string{"title": title.Title, "command": ""}, cmd)
	return nil
}

// commandName reads an LSP Command's command identifier ("" when absent).
func commandName(
	raw json.RawMessage,
) (string, error) {
	var cmd struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return "", fmt.Errorf("convert: command identifier: %w", err)
	}
	return cmd.Command, nil
}

// decodes reports whether raw has v's shape: a result of another shape is not
// one this package shapes.
func decodes(
	raw json.RawMessage,
	v any,
) bool {
	return json.Unmarshal(raw, v) == nil
}

func marshalOr(
	v any,
	fallback json.RawMessage,
) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return out
}
