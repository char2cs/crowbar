package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

type HealthHandler struct {
	usecase *usecases.HealthUsecase
}

func NewHealthHandler(uc *usecases.HealthUsecase) *HealthHandler {
	return &HealthHandler{usecase: uc}
}

func (h *HealthHandler) Check(c *gin.Context) {
	c.JSON(http.StatusOK, h.usecase.Check())
}
