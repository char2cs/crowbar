package health

import (
	"github.com/gin-gonic/gin"

	healthhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health/handlers"
)

func Register(
	rg *gin.RouterGroup,
	deadLetters healthhandlers.DeadLetters,
) {
	rg.GET("/health", healthhandlers.New(deadLetters).Check)
}
