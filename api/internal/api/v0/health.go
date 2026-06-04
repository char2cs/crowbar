package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func registerHealth(
	rg *gin.RouterGroup,
) {
	rg.GET("/health", healthHandler)
}

func healthHandler(
	c *gin.Context,
) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": metadata.GetVersion(),
	})
}
