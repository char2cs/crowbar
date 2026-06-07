package dto

import (
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// LSPPositionRequest is the body shared by the position-addressed LSP feature
// routes (completion, hover, definition, references): the workspace-relative
// file path and the zero-based cursor position.
type LSPPositionRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
}

// LSPRangeRequest is the body for the code-action route: the file path and the
// selection range the actions apply to.
type LSPRangeRequest struct {
	Path  string       `json:"path" binding:"required"`
	Range domlsp.Range `json:"range"`
}

// LSPRenameRequest is the body for the rename route: the file path, the symbol
// position, and the replacement identifier.
type LSPRenameRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
	NewName  string          `json:"newName" binding:"required"`
}

// LSPPathRequest is the body for the path-only LSP routes (document symbol and
// document close): the workspace-relative file path.
type LSPPathRequest struct {
	Path string `json:"path" binding:"required"`
}

// LSPDidOpenRequest is the body for the document-open notification: the file
// path, the language identifier, and the full buffer text.
type LSPDidOpenRequest struct {
	Path       string `json:"path" binding:"required"`
	LanguageID string `json:"languageId" binding:"required"`
	Text       string `json:"text"`
}

// LSPDidChangeRequest is the body for the document-change notification: the
// file path and the full replacement buffer text.
type LSPDidChangeRequest struct {
	Path string `json:"path" binding:"required"`
	Text string `json:"text"`
}
