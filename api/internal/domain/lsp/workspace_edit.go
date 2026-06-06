package lsp

// TextEdit replaces a Range in a file with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// WorkspaceEdit groups per-file text edits returned by rename/codeAction.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
}
