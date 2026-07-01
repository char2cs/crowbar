// Package editor mounts the v0 editor REST routes: git blame, the synchronous
// LSP feature requests, the document-sync notifications, and the diagnostics
// snapshot (02 §2.5, 04 §3, 10). Every route hangs off /workspaces/:wsId; blame
// carries its path in the ?path= query because GET has no body, while the LSP
// routes take their path in the JSON body. The diagnostics WebSocket stream is
// co-located on .../workspaces/:wsId/lsp/ws (W7-2).
package editor

import (
	"github.com/gin-gonic/gin"

	editorhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor/handlers"
)

// Register mounts the blame, LSP feature, document-sync, diagnostics routes,
// and the co-located .../lsp/ws WebSocket upgrade on the supplied router group,
// backed by the LSP host, the git engine, and the workspace reader. lspWSHandle
// is the pre-built broadcaster handle for the live diagnostics stream.
func Register(
	rg *gin.RouterGroup,
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
	wsReader editorhandlers.WorkspaceReader,
	lspWSHandle gin.HandlerFunc,
) {
	h := editorhandlers.New(lsp, git, wsReader)
	rg.GET("/workspaces/:wsId/blame", h.Blame)
	rg.POST("/workspaces/:wsId/lsp/completion", h.Completion)
	rg.POST("/workspaces/:wsId/lsp/hover", h.Hover)
	rg.POST("/workspaces/:wsId/lsp/definition", h.Definition)
	rg.POST("/workspaces/:wsId/lsp/references", h.References)
	rg.POST("/workspaces/:wsId/lsp/rename", h.Rename)
	rg.POST("/workspaces/:wsId/lsp/codeAction", h.CodeAction)
	rg.POST("/workspaces/:wsId/lsp/documentSymbol", h.DocumentSymbol)
	rg.GET("/workspaces/:wsId/lsp/diagnostics", h.Diagnostics)
	rg.POST("/workspaces/:wsId/lsp/didOpen", h.DidOpen)
	rg.POST("/workspaces/:wsId/lsp/didChange", h.DidChange)
	rg.POST("/workspaces/:wsId/lsp/didClose", h.DidClose)
	rg.GET("/workspaces/:wsId/lsp/ws", lspWSHandle)
}
