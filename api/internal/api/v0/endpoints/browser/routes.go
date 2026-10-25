// Package browser mounts the socket the desktop app attaches to so agents can
// drive its browser tabs.
package browser

import (
	"github.com/gin-gonic/gin"

	browserhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/browser/handlers"
	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

// Register mounts GET /browser/peer, a WebSocket carrying tab inventories up
// and agent requests down.
func Register(
	rg *gin.RouterGroup,
	bridge *browserbridge.Bridge,
) {
	rg.GET("/browser/peer", browserhandlers.New(bridge).Peer)
}
