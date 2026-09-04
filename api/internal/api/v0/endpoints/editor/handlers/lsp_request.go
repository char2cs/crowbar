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
	worktreePath, req, ok := h.bindPosition(c)
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
	worktreePath, req, ok := h.bindPosition(c)
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
	worktreePath, req, ok := h.bindPosition(c)
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
	worktreePath, req, ok := h.bindPosition(c)
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
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPRenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, edit)
}

// CodeAction handles POST /v0/chats/:chatId/lsp/codeAction. The data field
// carries the raw textDocument/codeAction result, passed through unchanged.
func (h *Handlers) CodeAction(
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
	result, err := h.lsp.CodeAction(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Range,
	)
	writeRaw(c, result, err)
}

// DocumentSymbol handles POST /v0/chats/:chatId/lsp/documentSymbol. The data
// field carries the raw textDocument/documentSymbol result, unchanged.
func (h *Handlers) DocumentSymbol(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
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

func (h *Handlers) bindPosition(
	c *gin.Context,
) (string, dto.LSPPositionRequest, bool) {
	var req dto.LSPPositionRequest
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if locations == nil {
		libs.WriteQueryOK(c, []domlsp.Location{})
		return
	}
	libs.WriteQueryOK(c, locations)
}
