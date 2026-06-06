// Package health mounts the v0 health-check route.
package health

import (
	"github.com/gin-gonic/gin"

	healthhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health/handlers"
)

// Register mounts GET /health on the supplied router group.
func Register(
	rg *gin.RouterGroup,
) {
	rg.GET("/health", healthhandlers.Check)
}
