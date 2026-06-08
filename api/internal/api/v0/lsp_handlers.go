package v0

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

type lspPositionRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
}

type lspRangeRequest struct {
	Path  string       `json:"path" binding:"required"`
	Range domlsp.Range `json:"range"`
}

type lspRenameRequest struct {
	Path     string          `json:"path" binding:"required"`
	Position domlsp.Position `json:"position"`
	NewName  string          `json:"newName" binding:"required"`
}

type lspPathRequest struct {
	Path string `json:"path" binding:"required"`
}

type lspDidOpenRequest struct {
	Path       string `json:"path" binding:"required"`
	LanguageID string `json:"languageId" binding:"required"`
	Text       string `json:"text"`
}

type lspDidChangeRequest struct {
	Path string `json:"path" binding:"required"`
	Text string `json:"text"`
}

// registerLSPHandlers mounts the LSP feature, diagnostics, and document-sync
// REST routes on rg (02 §2.5).
func registerLSPHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/lsp/completion", c.handleLSPCompletion)
	rg.POST("/workspaces/:wsId/lsp/hover", c.handleLSPHover)
	rg.POST("/workspaces/:wsId/lsp/definition", c.handleLSPDefinition)
	rg.POST("/workspaces/:wsId/lsp/references", c.handleLSPReferences)
	rg.POST("/workspaces/:wsId/lsp/rename", c.handleLSPRename)
	rg.POST("/workspaces/:wsId/lsp/codeAction", c.handleLSPCodeAction)
	rg.POST("/workspaces/:wsId/lsp/documentSymbol", c.handleLSPDocumentSymbol)
	rg.GET("/workspaces/:wsId/lsp/diagnostics", c.handleLSPDiagnostics)
	rg.POST("/workspaces/:wsId/lsp/didOpen", c.handleLSPDidOpen)
	rg.POST("/workspaces/:wsId/lsp/didChange", c.handleLSPDidChange)
	rg.POST("/workspaces/:wsId/lsp/didClose", c.handleLSPDidClose)
}

func (c *Container) handleLSPCompletion(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspPositionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	result, err := c.eng.LSP.Completion(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Position)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, lspRaw(result))
}

func (c *Container) handleLSPHover(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspPositionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	result, err := c.eng.LSP.Hover(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Position)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, lspRaw(result))
}

func (c *Container) handleLSPDefinition(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspPositionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	locations, err := c.eng.LSP.Definition(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Position)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, lspLocations(locations))
}

func (c *Container) handleLSPReferences(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspPositionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	locations, err := c.eng.LSP.References(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Position)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, lspLocations(locations))
}

func (c *Container) handleLSPRename(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspRenameRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	edit, err := c.eng.LSP.Rename(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Position, req.NewName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, edit)
}

func (c *Container) handleLSPCodeAction(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspRangeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	result, err := c.eng.LSP.CodeAction(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Range)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, lspRaw(result))
}

func (c *Container) handleLSPDocumentSymbol(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspPathRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	result, err := c.eng.LSP.DocumentSymbol(ctx.Request.Context(), wsID, worktreePath, req.Path)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, lspRaw(result))
}

func (c *Container) handleLSPDiagnostics(
	ctx *gin.Context,
) {
	if _, ok := c.lspWorkspacePath(ctx); !ok {
		return
	}
	wsID := ctx.Param("wsId")
	diags := c.eng.LSP.DiagnosticsSnapshot(wsID)
	ctx.JSON(http.StatusOK, domlsp.DiagnosticsEvent{WsID: wsID, Diagnostics: diags})
}

func (c *Container) handleLSPDidOpen(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspDidOpenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	err := c.eng.LSP.DidOpen(ctx.Request.Context(), wsID, worktreePath, req.Path, req.LanguageID, req.Text)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}

func (c *Container) handleLSPDidChange(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspDidChangeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	if err := c.eng.LSP.DidChange(ctx.Request.Context(), wsID, worktreePath, req.Path, req.Text); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}

func (c *Container) handleLSPDidClose(
	ctx *gin.Context,
) {
	worktreePath, ok := c.lspWorkspacePath(ctx)
	if !ok {
		return
	}
	var req lspPathRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := ctx.Param("wsId")
	if err := c.eng.LSP.DidClose(ctx.Request.Context(), wsID, worktreePath, req.Path); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}

func lspRaw(
	raw json.RawMessage,
) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}

func lspLocations(
	locations []domlsp.Location,
) []domlsp.Location {
	if locations == nil {
		return []domlsp.Location{}
	}
	return locations
}

func (c *Container) lspWorkspacePath(
	ctx *gin.Context,
) (string, bool) {
	wsID := ctx.Param("wsId")
	row, err := c.app.Repositories.Workspace.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return "", false
	}
	return row.WorktreePath, true
}
