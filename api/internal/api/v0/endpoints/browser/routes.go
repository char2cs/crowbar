package browser

import (
	"github.com/gin-gonic/gin"

	browserhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/browser/handlers"
	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

func Register(
	rg *gin.RouterGroup,
	bridge *browserbridge.Bridge,
) {
	rg.GET("/browser/peer", browserhandlers.New(bridge).Peer)
}
