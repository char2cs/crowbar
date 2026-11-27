package identity

import (
	"github.com/gin-gonic/gin"

	identityhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	identity identityhandlers.IdentityResolver,
) {
	h := identityhandlers.New(identity)
	chatScoped.GET("/identity", h.Get)
}
