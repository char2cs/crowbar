package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// SignatureHelp handles POST /v0/chats/:chatId/lsp/signatureHelp. The data
// field carries the raw textDocument/signatureHelp result, or null.
func (h *Handlers) SignatureHelp(
	c *gin.Context,
) {
	worktreePath, req, ok := h.bindPosition(c)
	if !ok {
		return
	}
	result, err := h.lsp.SignatureHelp(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Position,
	)
	writeRaw(c, result, err)
}

// CodeLens handles POST /v0/chats/:chatId/lsp/codeLens. The data field carries
// the raw textDocument/codeLens result, or null.
func (h *Handlers) CodeLens(
	c *gin.Context,
) {
	worktreePath, req, ok := h.bindPath(c)
	if !ok {
		return
	}
	result, err := h.lsp.CodeLens(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
	)
	writeRaw(c, result, err)
}

// CodeLensResolve handles POST /v0/chats/:chatId/lsp/codeLensResolve. The
// body carries a lens CodeLens returned; the data field carries the resolved
// lens.
func (h *Handlers) CodeLensResolve(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPCodeLensResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.lsp.CodeLensResolve(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Lens,
	)
	writeRaw(c, result, err)
}

// SemanticTokens handles POST /v0/chats/:chatId/lsp/semanticTokens. The data
// field carries the file's semantic tokens in the canonical legend — a delta
// ({resultId, edits}) when previousResultId is set and the server supports
// one, else the full set ({resultId, data}) — or null when none are offered.
func (h *Handlers) SemanticTokens(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPSemanticTokensRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.lsp.SemanticTokens(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.PreviousResultID,
	)
	writeRaw(c, result, err)
}

// SemanticTokensRange handles POST /v0/chats/:chatId/lsp/semanticTokensRange.
// The data field carries the range's tokens ({data}) in the canonical legend,
// or null when the server offers no range tokens.
func (h *Handlers) SemanticTokensRange(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPRangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.lsp.SemanticTokensRange(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Range,
	)
	writeRaw(c, result, err)
}

// ExecuteCommand handles POST /v0/chats/:chatId/lsp/executeCommand: it runs a
// command a code lens or code action carried. The data field carries the
// server's result and the workspace edits the command applied, for the
// editor to apply.
func (h *Handlers) ExecuteCommand(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPExecuteCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.lsp.ExecuteCommand(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Command,
		req.Arguments,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, result)
}

// Formatting handles POST /v0/chats/:chatId/lsp/formatting. The data field
// carries the raw textDocument/formatting result (TextEdit[]), or null.
func (h *Handlers) Formatting(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPFormattingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.lsp.Formatting(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Options,
	)
	writeRaw(c, result, err)
}

// DidSave handles POST /v0/chats/:chatId/lsp/didSave, forwarding a
// textDocument/didSave notification for an open document.
func (h *Handlers) DidSave(
	c *gin.Context,
) {
	worktreePath, req, ok := h.bindPath(c)
	if !ok {
		return
	}
	err := h.lsp.DidSave(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}

// Status handles GET /v0/chats/:chatId/lsp/status?path=. The data field
// carries the ServerStatus of the server for the file's language.
func (h *Handlers) Status(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	if _, ok := h.worktreePath(c); !ok {
		return
	}
	path := c.Query("path")
	if path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	libs.WriteQueryOK(c, h.lsp.Status(h.lspOwnerID(c), path))
}

// Restart handles POST /v0/chats/:chatId/lsp/restart. It restarts the running
// server for the file's language (reopening its documents) and answers with
// the resulting ServerStatus. A server that is not running is not spawned.
func (h *Handlers) Restart(
	c *gin.Context,
) {
	if _, req, ok := h.bindPath(c); ok {
		status, err := h.lsp.Restart(c.Request.Context(), h.lspOwnerID(c), req.Path)
		if err != nil {
			code, msg := libs.StatusAndMessage(err)
			libs.WriteErr(c, code, msg)
			return
		}
		libs.WriteQueryOK(c, status)
	}
}

func (h *Handlers) bindPath(
	c *gin.Context,
) (string, dto.LSPPathRequest, bool) {
	var req dto.LSPPathRequest
	if !h.requireLSP(c) {
		return "", req, false
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return "", req, false
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return "", req, false
	}
	return worktreePath, req, true
}
