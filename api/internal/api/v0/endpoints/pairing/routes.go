package pairing

import (
	"github.com/gin-gonic/gin"

	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
)

func RegisterOpen(
	open *gin.RouterGroup,
	h *pairinghandlers.Handlers,
	redeemLimiter gin.HandlerFunc,
) {
	open.POST("/pair", redeemLimiter, h.Pair)
}

func RegisterGuarded(
	guarded *gin.RouterGroup,
	h *pairinghandlers.Handlers,
) {
	guarded.POST("/pair/code", h.MintCode)
	guarded.GET("/clients", h.ListClients)
	guarded.DELETE("/clients/:id", h.RevokeClient)
}
