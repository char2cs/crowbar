package documents

import (
	"github.com/gin-gonic/gin"
)

func Register(
	rg *gin.RouterGroup,
	files Documents,
) {
	h := &handlers{docs: files}
	rg.GET("/documents/content", h.Read)
	rg.PUT("/documents/content", h.Write)
}
