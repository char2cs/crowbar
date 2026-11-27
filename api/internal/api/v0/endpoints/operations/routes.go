package operations

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/operations/handlers"
)

func Register(
	rg *gin.RouterGroup,
	ops handlers.Dismisser,
) {
	rg.DELETE("/operations/:id", handlers.New(ops).Dismiss)
}
