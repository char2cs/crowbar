package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

type syncOK struct {
	OK bool `json:"ok"`
}

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
