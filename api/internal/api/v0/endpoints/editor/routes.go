package editor

import (
	"github.com/gin-gonic/gin"

	editorhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
) {
	h := editorhandlers.New(lsp, git)
	mount(chatScoped, "", h)
}

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
