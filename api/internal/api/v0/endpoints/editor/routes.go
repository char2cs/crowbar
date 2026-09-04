// Package editor mounts the v0 editor REST routes: git blame, the synchronous
// LSP feature requests, the document-sync notifications, and the diagnostics
// snapshot (02 §2.5, 04 §3, 10). Every route carries blame's path in the
// ?path= query because GET has no body, while the LSP routes take their path
// in the JSON body. The diagnostics WebSocket stream is co-located at
// .../lsp/ws (W7-2).
package editor

import (
	"github.com/gin-gonic/gin"

	editorhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor/handlers"
)

// Register mounts the editor REST + WS surface on the flat chat-scoped group
// (spec §7.1), the only surface editor/LSP is addressable through:
// /v0/chats/:chatId/{blame,lsp/*} .
//
// editor/LSP is spec §4.2's OWNED bucket: the resolver still runs, for a CWD,
// but the session itself is never shared with a sibling chat that happens to
// hold the same worktree (spec law 5). The worktree is resolved from the
// request context by chatScoped's own resolveChatWorktree middleware (see
// handlers.Handlers.workspace), and the LSP engine calls key their session by
// the chat id (handlers.Handlers.lspOwnerID), so a sibling chat sharing this
// worktree gets its own LSP session.
//
// The old /projects/:projectId/repos/:repoId/workspaces/:wsId/{blame,lsp/*}
// mount is gone (spec §8 step 6): every caller had already moved to the mount
// kept here.
//
// lspWSHandle is the pre-built broadcaster handle for the live diagnostics
// stream, dual-mounted the same way as the REST routes.
func Register(
	chatScoped *gin.RouterGroup,
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
	lspWSHandle gin.HandlerFunc,
) {
	h := editorhandlers.New(lsp, git)
	mount(chatScoped, "", h, lspWSHandle)
}

// mount registers the 13-route editor surface under prefix on rg.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *editorhandlers.Handlers,
	lspWSHandle gin.HandlerFunc,
) {
	rg.GET(prefix+"/blame", h.Blame)
	rg.POST(prefix+"/lsp/completion", h.Completion)
	rg.POST(prefix+"/lsp/hover", h.Hover)
	rg.POST(prefix+"/lsp/definition", h.Definition)
	rg.POST(prefix+"/lsp/references", h.References)
	rg.POST(prefix+"/lsp/rename", h.Rename)
	rg.POST(prefix+"/lsp/codeAction", h.CodeAction)
	rg.POST(prefix+"/lsp/documentSymbol", h.DocumentSymbol)
	rg.GET(prefix+"/lsp/diagnostics", h.Diagnostics)
	rg.POST(prefix+"/lsp/didOpen", h.DidOpen)
	rg.POST(prefix+"/lsp/didChange", h.DidChange)
	rg.POST(prefix+"/lsp/didClose", h.DidClose)
	rg.GET(prefix+"/lsp/ws", lspWSHandle)
}
