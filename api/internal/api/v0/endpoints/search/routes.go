package search

import (
	"github.com/gin-gonic/gin"

	searchhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	searchEng searchhandlers.SearchEngine,
) {
	h := searchhandlers.New(searchEng)
	mount(chatScoped, "/search", h)
}

func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *searchhandlers.Handlers,
) {
	rg.POST(prefix, h.Search)
	rg.POST(prefix+"/replace", h.Replace)
}
