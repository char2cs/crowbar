package review

import (
	"github.com/gin-gonic/gin"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	reviewUsecase reviewhandlers.ReviewUsecase,
) {
	h := reviewhandlers.New(reviewUsecase)
	mount(chatScoped, "/review", h)
}

func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *reviewhandlers.Handlers,
) {
	rg.GET(prefix, h.Get)
	rg.GET(prefix+"/files", h.GetFiles)
	rg.GET(prefix+"/outline", h.GetOutline)
	rg.GET(prefix+"/patch", h.GetPatch)
	rg.GET(prefix+"/search", h.SearchDiff)
	rg.GET(prefix+"/contents", h.GetFileSides)
	rg.PATCH(prefix, h.SetMergeStrategy)
}
