package dto

import (
	"encoding/json"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

type LSPPositionRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
}

type LSPRangeRequest struct {
	Path        string          `json:"path" binding:"required"`
	Range       domlsp.Range    `json:"range"`
	Diagnostics json.RawMessage `json:"diagnostics,omitempty"`
}

type LSPFormattingRequest struct {
	Path    string                   `json:"path" binding:"required"`
	Options domlsp.FormattingOptions `json:"options"`
}

type LSPCodeLensResolveRequest struct {
	Path string          `json:"path" binding:"required"`
	Lens json.RawMessage `json:"lens" binding:"required"`
}

type LSPSemanticTokensRequest struct {
	Path             string `json:"path" binding:"required"`
	PreviousResultID string `json:"previousResultId"`
}

type LSPExecuteCommandRequest struct {
	Path      string          `json:"path" binding:"required"`
	Command   string          `json:"command" binding:"required"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type LSPRenameRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
	NewName  string          `json:"newName" binding:"required"`
}

type LSPPathRequest struct {
	Path string `json:"path" binding:"required"`
}

type LSPDidOpenRequest struct {
	Path       string `json:"path" binding:"required"`
	LanguageID string `json:"languageId" binding:"required"`
	Text       string `json:"text"`
}

type LSPDidChangeRequest struct {
	Path string `json:"path" binding:"required"`
	Text string `json:"text"`
}
