// Package editor mounts the v0 editor routes: blame, LSP features, document sync, diagnostics.
// Blame takes its path in ?path= because GET has no body; the LSP routes take it in the JSON
// body. Live diagnostics ride the stream's lsp: scope.
package editor

import (
	"github.com/gin-gonic/gin"

	editorhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor/handlers"
)

// Register mounts the editor routes at /v0/chats/:chatId/{blame,lsp/*}. The
// worktree comes from chatScoped's resolveChatWorktree middleware, but LSP
// sessions are keyed by chat id so sibling chats sharing a worktree never share one.
func Register(
	chatScoped *gin.RouterGroup,
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
) {
	h := editorhandlers.New(lsp, git)
	mount(chatScoped, "", h)
}

// mount registers the editor surface under prefix on rg.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *editorhandlers.Handlers,
) {
	rg.GET(prefix+"/blame", h.Blame)
	rg.POST(prefix+"/lsp/completion", h.Completion)
	rg.POST(prefix+"/lsp/hover", h.Hover)
	rg.POST(prefix+"/lsp/definition", h.Definition)
	rg.POST(prefix+"/lsp/references", h.References)
	rg.POST(prefix+"/lsp/rename", h.Rename)
	rg.POST(prefix+"/lsp/codeAction", h.CodeAction)
	rg.POST(prefix+"/lsp/documentSymbol", h.DocumentSymbol)
	rg.POST(prefix+"/lsp/signatureHelp", h.SignatureHelp)
	rg.POST(prefix+"/lsp/codeLens", h.CodeLens)
	rg.POST(prefix+"/lsp/codeLensResolve", h.CodeLensResolve)
	rg.POST(prefix+"/lsp/semanticTokens", h.SemanticTokens)
	rg.POST(prefix+"/lsp/semanticTokensRange", h.SemanticTokensRange)
	rg.POST(prefix+"/lsp/executeCommand", h.ExecuteCommand)
	rg.POST(prefix+"/lsp/formatting", h.Formatting)
	rg.GET(prefix+"/lsp/status", h.Status)
	rg.POST(prefix+"/lsp/restart", h.Restart)
	rg.GET(prefix+"/lsp/diagnostics", h.Diagnostics)
	rg.POST(prefix+"/lsp/didOpen", h.DidOpen)
	rg.POST(prefix+"/lsp/didChange", h.DidChange)
	rg.POST(prefix+"/lsp/didSave", h.DidSave)
	rg.POST(prefix+"/lsp/didClose", h.DidClose)
}
