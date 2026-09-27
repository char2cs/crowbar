// Package pairing mounts the pairing and client-registry routes.
package pairing

import (
	"github.com/gin-gonic/gin"

	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
)

// RegisterOpen mounts the routes that must stay reachable WITHOUT the
// bearer-auth guard: minting and redeeming are how an unpaired client
// becomes paired, so they cannot require the very credential they produce.
// redeemLimiter guards POST /pair alone — there is no client identity to key
// on before pairing succeeds, so a wrong-code guesser is capped globally
// without capping the health probe every saved host runs continuously.
//
// The caller (router.go) MUST call this BEFORE installing requireBearerAuth
// on the same underlying group, and call RegisterGuarded AFTER: gin snapshots
// a group's middleware chain at route-registration time, so a route
// registered here never carries middleware added to the group later. This is
// what lets an unpaired client redeem a code at all.
func RegisterOpen(
	open *gin.RouterGroup,
	h *pairinghandlers.Handlers,
	redeemLimiter gin.HandlerFunc,
) {
	open.POST("/pair/code", h.MintCode)
	open.POST("/pair", redeemLimiter, h.Pair)
}

// RegisterGuarded mounts the client-registry routes, which — like every other
// v0 route — require a valid bearer token once requireBearerAuth is installed
// on guarded (see RegisterOpen's doc comment for why the caller's ordering
// matters).
func RegisterGuarded(
	guarded *gin.RouterGroup,
	h *pairinghandlers.Handlers,
) {
	guarded.GET("/clients", h.ListClients)
	guarded.DELETE("/clients/:id", h.RevokeClient)
}
