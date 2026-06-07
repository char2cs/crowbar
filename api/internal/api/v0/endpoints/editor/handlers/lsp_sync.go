package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// syncOK is the data shape returned by the document-sync notifications: a fixed
// acknowledgement that the engine accepted the notification.
type syncOK struct {
	OK bool `json:"ok"`
}

// DidOpen handles POST /v0/workspaces/:wsId/lsp/didOpen, forwarding a
// textDocument/didOpen notification with the full buffer text.
func (h *Handlers) DidOpen(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPDidOpenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	err := h.lsp.DidOpen(
		c.Request.Context(),
		c.Param("wsId"),
		worktreePath,
		req.Path,
		req.LanguageID,
		req.Text,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}

// DidChange handles POST /v0/workspaces/:wsId/lsp/didChange, forwarding a
// textDocument/didChange notification with the full replacement buffer text.
func (h *Handlers) DidChange(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	var req dto.LSPDidChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	err := h.lsp.DidChange(
		c.Request.Context(),
		c.Param("wsId"),
		worktreePath,
		req.Path,
		req.Text,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}

// DidClose handles POST /v0/workspaces/:wsId/lsp/didClose, forwarding a
// textDocument/didClose notification.
func (h *Handlers) DidClose(
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
	err := h.lsp.DidClose(
		c.Request.Context(),
		c.Param("wsId"),
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
