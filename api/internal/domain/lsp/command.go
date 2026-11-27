package lsp

import "encoding/json"

type CommandResult struct {
	Result json.RawMessage   `json:"result"`
	Edits  []json.RawMessage `json:"edits"`
}
