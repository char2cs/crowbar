package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

func (h *Handlers) SignatureHelp(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPositionRequest](h, c)
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

func (h *Handlers) CodeLens(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPathRequest](h, c)
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

func (h *Handlers) CodeLensResolve(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPCodeLensResolveRequest](h, c)
	if !ok {
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

func (h *Handlers) SemanticTokens(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPSemanticTokensRequest](h, c)
	if !ok {
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

func (h *Handlers) SemanticTokensRange(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPRangeRequest](h, c)
	if !ok {
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

func (h *Handlers) ExecuteCommand(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPExecuteCommandRequest](h, c)
	if !ok {
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
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, result)
}

func (h *Handlers) Formatting(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPFormattingRequest](h, c)
	if !ok {
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

func (h *Handlers) DidSave(
	c *gin.Context,
) {
	worktreePath, req, ok := bindLSP[dto.LSPPathRequest](h, c)
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
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, syncOK{OK: true})
}

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

func (h *Handlers) Restart(
	c *gin.Context,
) {
	if _, req, ok := bindLSP[dto.LSPPathRequest](h, c); ok {
		status, err := h.lsp.Restart(c.Request.Context(), h.lspOwnerID(c), req.Path)
		if err != nil {
			libs.Fail(c, err)
			return
		}
		libs.WriteQueryOK(c, status)
	}
}
