package v0

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// rejectEmptyPathParams guards every v0 route against empty path parameters.
//
// gin's radix tree happily matches a request like GET /v0/workspaces//chats
// against /v0/workspaces/:wsId/chats with wsId == "" — the empty segment
// between the two slashes binds the param. Handlers would then pass "" to
// repositories and usecases as if it were a real id (list queries return data
// scoped to a nonexistent workspace, lookups 404 confusingly, etc.). Reject
// such requests up front with a 400 error envelope.
func rejectEmptyPathParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, p := range c.Params {
			if p.Value == "" {
				libs.WriteErr(
					c,
					http.StatusBadRequest,
					fmt.Sprintf("path parameter %q must not be empty", p.Key),
				)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
