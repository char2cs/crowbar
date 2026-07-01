// Package identity mounts the v0 current-identity REST route. The endpoint
// resolves the current human's GitHub/git identity (login, displayName,
// avatarUrl) for attributing review comments and other authored actions.
package identity

import (
	"github.com/gin-gonic/gin"

	identityhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity/handlers"
)

// Register mounts the identity GET route on the supplied router group.
func Register(
	rg *gin.RouterGroup,
	identity identityhandlers.IdentityResolver,
	wsReader identityhandlers.WorkspaceReader,
) {
	h := identityhandlers.New(identity, wsReader)
	rg.GET("/workspaces/:wsId/identity", h.Get)
}
