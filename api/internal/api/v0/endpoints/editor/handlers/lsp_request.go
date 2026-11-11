package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// Completion handles POST /v0/chats/:chatId/lsp/completion. The data field
// carries the raw textDocument/completion result, passed through unchanged, or
// null when no server serves the file's language.
func (h *Handlers) Completion(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPositionRequest](h, c)
	if !ok {
		return
	}
	result, err := h.lsp.Completion(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Position,
	)
	writeRaw(c, result, err)
}

// Hover handles POST /v0/chats/:chatId/lsp/hover. The data field carries the
// raw textDocument/hover result, passed through unchanged, or null when absent.
func (h *Handlers) Hover(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPositionRequest](h, c)
	if !ok {
		return
	}
	result, err := h.lsp.Hover(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Position,
	)
	writeRaw(c, result, err)
}

// Definition handles POST /v0/chats/:chatId/lsp/definition. The data field
// carries the resolved locations as an array, empty when none resolve.
func (h *Handlers) Definition(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPositionRequest](h, c)
	if !ok {
		return
	}
	locations, err := h.lsp.Definition(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Position,
	)
	writeLocations(c, locations, err)
}

// References handles POST /v0/chats/:chatId/lsp/references. The data field
// carries the reference locations as an array, empty when none resolve.
func (h *Handlers) References(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPositionRequest](h, c)
	if !ok {
		return
	}
	locations, err := h.lsp.References(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Position,
	)
	writeLocations(c, locations, err)
}

// Rename handles POST /v0/chats/:chatId/lsp/rename. The data field carries
// the resulting workspace edit.
func (h *Handlers) Rename(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPRenameRequest](h, c)
	if !ok {
		return
	}
	edit, err := h.lsp.Rename(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Position,
		req.NewName,
	)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, edit)
}

// CodeAction handles POST /v0/chats/:chatId/lsp/codeAction. The data field
// carries the raw textDocument/codeAction result, passed through unchanged.
func (h *Handlers) CodeAction(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPRangeRequest](h, c)
	if !ok {
		return
	}
	result, err := h.lsp.CodeAction(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Range,
		req.Diagnostics,
	)
	writeRaw(c, result, err)
}

// DocumentSymbol handles POST /v0/chats/:chatId/lsp/documentSymbol. The data
// field carries the raw textDocument/documentSymbol result, unchanged.
func (h *Handlers) DocumentSymbol(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPathRequest](h, c)
	if !ok {
		return
	}
	result, err := h.lsp.DocumentSymbol(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
	)
	writeRaw(c, result, err)
}

// bindLSP answers the worktree path and the bound request body, having already
// written the response when the LSP is off, the worktree is unknown or the body is malformed.
func bindLSP[T any](h *Handlers, c *gin.Context) (string, T, bool) {
	var req T
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

func writeRaw(
	c *gin.Context,
	raw json.RawMessage,
	err error,
) {
	if err != nil {
		libs.Fail(c, err)
		return
	}
	if len(raw) == 0 {
		libs.WriteQueryOK(c, json.RawMessage("null"))
		return
	}
	libs.WriteQueryOK(c, raw)
}

func writeLocations(
	c *gin.Context,
	locations []domlsp.Location,
	err error,
) {
	if err != nil {
		libs.Fail(c, err)
		return
	}
	if locations == nil {
		libs.WriteQueryOK(c, []domlsp.Location{})
		return
	}
	libs.WriteQueryOK(c, locations)
}
