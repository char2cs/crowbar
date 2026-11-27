package files

import (
	"github.com/gin-gonic/gin"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	files filehandlers.Files,
) {
	h := filehandlers.New(files)
	mount(chatScoped, "/files", h)
}

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
