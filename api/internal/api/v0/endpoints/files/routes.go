// Package files mounts the v0 chat-scoped file REST routes.
package files

import (
	"github.com/gin-gonic/gin"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
)

// Register mounts /v0/chats/:chatId/files/... . Files are shared by every chat
// holding the worktree; the project home has no chat, so home.Register serves
// its files from its own handlers.
func Register(
	chatScoped *gin.RouterGroup,
	files filehandlers.Files,
) {
	h := filehandlers.New(files)
	mount(chatScoped, "/files", h)
}

// mount registers the files surface under prefix on rg. A file's
// content is read and written through /v0/documents/content.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *filehandlers.Handlers,
) {
	rg.GET(prefix+"/tree", h.Tree)
	rg.POST(prefix, h.Create)
	rg.POST(prefix+"/copy", h.Copy)
	rg.PATCH(prefix, h.Rename)
	rg.DELETE(prefix, h.Delete)
}
