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

// Register mounts the editor REST + WS surface on BOTH scoping groups it is
// currently addressable through.
//
// editor/LSP is spec §4.2's OWNED bucket: the resolver still runs, for a CWD,
// but the session itself is never shared with a sibling chat that happens to
// hold the same worktree (spec law 5). chatScoped is where it lives from now
// on — /v0/chats/:chatId/{blame,lsp/*} , the flat prefix §7.1 closes on — and
// the frontend talks to it exclusively.
//
// wsScoped is the OLD
// /projects/:projectId/repos/:repoId/workspaces/:wsId/{blame,lsp/*} surface,
// mounted unchanged. It is not a fallback and nothing chooses between the two:
// it is simply a route that has not been retired yet, and retiring it is spec
// §8 step 6's job. Deleting THIS call is the whole of editor's share of that
// step.
//
// One route table serves both groups here, so the two can never drift into
// different surfaces: mount is called twice with different prefixes, and a
// route added to it appears on both by construction. The handlers themselves
// take the worktree from whichever mount the request arrived on
// (handlers.Handlers.workspace), and the LSP engine calls key their session by
// whichever mount too (handlers.Handlers.lspOwnerID) — the chat id on the new
// mount, so a sibling chat sharing this worktree gets its own LSP session, and
// the workspace id on the old mount, unchanged.
//
// lspWSHandle is the pre-built broadcaster handle for the live diagnostics
// stream, dual-mounted the same way as the REST routes.
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
	wsReader editorhandlers.WorkspaceReader,
	lspWSHandle gin.HandlerFunc,
) {
	h := editorhandlers.New(lsp, git, wsReader)
	mount(chatScoped, "", h, lspWSHandle)
	mount(wsScoped, "/workspaces/:wsId", h, lspWSHandle)
}

// mount registers the 13-route editor surface under prefix on rg. It is the
// single definition of that surface; Register calls it once per live scoping
// group.
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
