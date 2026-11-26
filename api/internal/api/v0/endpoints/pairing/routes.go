// Package pairing mounts the pairing and client-registry routes.
package pairing

import (
	"github.com/gin-gonic/gin"

	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
)

// RegisterOpen mounts redeem, which produces the bearer credential and so must
// be registered before requireBearerAuth is installed. redeemLimiter caps
// POST /pair per source address: before pairing there is no client identity.
func RegisterOpen(
	open *gin.RouterGroup,
	h *pairinghandlers.Handlers,
	redeemLimiter gin.HandlerFunc,
) {
	open.POST("/pair", redeemLimiter, h.Pair)
}

// RegisterGuarded mounts mint and the client-registry routes; register them after
// requireBearerAuth is installed so they carry it.
func RegisterGuarded(
	guarded *gin.RouterGroup,
	h *pairinghandlers.Handlers,
) {
	guarded.POST("/pair/code", h.MintCode)
	guarded.GET("/clients", h.ListClients)
	guarded.DELETE("/clients/:id", h.RevokeClient)
}
