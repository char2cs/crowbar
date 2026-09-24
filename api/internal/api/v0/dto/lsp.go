package dto

import (
	"encoding/json"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// LSPPositionRequest is the body shared by the position-addressed LSP feature
// routes (completion, hover, definition, references): the workspace-relative
// file path and the zero-based cursor position.
type LSPPositionRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
}

// LSPRangeRequest is the body for the code-action route: the file path, the
// selection range the actions apply to, and (optionally) the LSP Diagnostics
// in that range the actions should address.
type LSPRangeRequest struct {
	Path        string          `json:"path" binding:"required"`
	Range       domlsp.Range    `json:"range"`
	Diagnostics json.RawMessage `json:"diagnostics,omitempty"`
}

// LSPFormattingRequest is the body for the document-formatting route.
type LSPFormattingRequest struct {
	Path    string                   `json:"path" binding:"required"`
	Options domlsp.FormattingOptions `json:"options"`
}

// LSPCodeLensResolveRequest is the body for the code-lens resolve route: the
// file the lens belongs to and the raw lens to resolve.
type LSPCodeLensResolveRequest struct {
	Path string          `json:"path" binding:"required"`
	Lens json.RawMessage `json:"lens" binding:"required"`
}

// LSPRenameRequest is the body for the rename route: the file path, the symbol
// position, and the replacement identifier.
type LSPRenameRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
	NewName  string          `json:"newName" binding:"required"`
}

// LSPPathRequest is the body for the path-only LSP routes (document symbol,
// code lens, document save/close, restart): the workspace-relative file path.
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
