package lsp

import "encoding/json"

// CommandResult is the outcome of a workspace/executeCommand: the server's raw
// result and the workspace edits (workspace-relative, raw LSP WorkspaceEdits)
// it asked the editor to apply while the command ran.
type CommandResult struct {
	Result json.RawMessage   `json:"result"`
	Edits  []json.RawMessage `json:"edits"`
}
