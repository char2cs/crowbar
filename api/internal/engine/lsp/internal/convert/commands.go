package convert

import "encoding/json"

// ClientCodeActions shapes a textDocument/codeAction result for the editor:
// every workspace edit names workspace-relative files, and only commands the
// server itself runs (canRun) survive — a bare command it cannot run is
// dropped, and an action's unrunnable command is stripped, keeping its edit.
// A result that does not decode is passed through unchanged.
func ClientCodeActions(
	worktreePath string,
	raw json.RawMessage,
	canRun func(string) bool,
) json.RawMessage {
	var actions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &actions); err != nil {
		return raw
	}
	kept := make([]map[string]json.RawMessage, 0, len(actions))
	for _, action := range actions {
		if clientCodeAction(worktreePath, action, canRun) {
			kept = append(kept, action)
		}
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return raw
	}
	return out
}

// clientCodeAction rewrites one Command | CodeAction in place and reports
// whether the editor can still do anything with it.
func clientCodeAction(
	worktreePath string,
	action map[string]json.RawMessage,
	canRun func(string) bool,
) bool {
	var bare string
	if json.Unmarshal(action["command"], &bare) == nil && bare != "" {
		return canRun(bare)
	}
	if cmd, ok := action["command"]; ok && !canRun(commandName(cmd)) {
		delete(action, "command")
	}
	if edit, ok := action["edit"]; ok {
		action["edit"] = RelWorkspaceEdit(worktreePath, edit)
	}
	_, hasEdit := action["edit"]
	_, hasCommand := action["command"]
	return hasEdit || hasCommand
}

// ClientLenses shapes a textDocument/codeLens result (an array) or a
// codeLens/resolve result (one lens) for the editor: a lens whose command the
// server cannot run keeps its title but loses the command, so the editor
// shows it as a label rather than a button that fails.
func ClientLenses(
	raw json.RawMessage,
	canRun func(string) bool,
) json.RawMessage {
	var lenses []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &lenses); err == nil {
		for _, lens := range lenses {
			clientLens(lens, canRun)
		}
		return marshalOr(lenses, raw)
	}
	var lens map[string]json.RawMessage
	if err := json.Unmarshal(raw, &lens); err != nil || lens == nil {
		return raw
	}
	clientLens(lens, canRun)
	return marshalOr(lens, raw)
}

func clientLens(
	lens map[string]json.RawMessage,
	canRun func(string) bool,
) {
	cmd, ok := lens["command"]
	if !ok || canRun(commandName(cmd)) {
		return
	}
	var title struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(cmd, &title)
	lens["command"] = marshalOr(map[string]string{"title": title.Title, "command": ""}, cmd)
}

// commandName reads an LSP Command's command identifier ("" when absent).
func commandName(
	raw json.RawMessage,
) string {
	var cmd struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(raw, &cmd)
	return cmd.Command
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
