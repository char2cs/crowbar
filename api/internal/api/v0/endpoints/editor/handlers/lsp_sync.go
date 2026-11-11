package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// syncOK is the data shape returned by the document-sync notifications: a fixed
// acknowledgement that the engine accepted the notification.
type syncOK struct {
	OK bool `json:"ok"`
}

// DidOpen handles POST /v0/chats/:chatId/lsp/didOpen, forwarding a
// textDocument/didOpen notification with the full buffer text.
func (h *Handlers) DidOpen(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPDidOpenRequest](h, c)
	if !ok {
		return
	}
	err := h.lsp.DidOpen(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.LanguageID,
		req.Text,
	)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}

// DidChange handles POST /v0/chats/:chatId/lsp/didChange, forwarding a
// textDocument/didChange notification with the full replacement buffer text.
func (h *Handlers) DidChange(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPDidChangeRequest](h, c)
	if !ok {
		return
	}
	err := h.lsp.DidChange(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
		req.Text,
	)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}

// DidClose handles POST /v0/chats/:chatId/lsp/didClose, forwarding a
// textDocument/didClose notification.
func (h *Handlers) DidClose(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPathRequest](h, c)
	if !ok {
		return
	}
	err := h.lsp.DidClose(
		c.Request.Context(),
		h.lspOwnerID(c),
		worktreePath,
		req.Path,
	)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}
