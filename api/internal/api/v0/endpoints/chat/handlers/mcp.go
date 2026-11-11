package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// MCP handles POST .../chats/runners/:segid/mcp: {"token","rpc"} in, data.rpc out, 204 for a
// notification. The rpc is forwarded raw so a protocol revision the handler predates is never
// corrupted. Scope comes from the token-authenticated runner, never the URL, so there is no chat check.
func (h *Handlers) MCP(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	segID := ctx.Param("segid")

	var body struct {
		Token string          `json:"token"`
		RPC   json.RawMessage `json:"rpc"`
	}
	// An absent rpc member is a malformed request, not a notification: answering
	// it with the notification's 204 would hide the caller's bug behind a success.
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.RPC) == 0 {
		libs.WriteErr(ctx, http.StatusBadRequest, "body must carry an rpc message")
		return
	}

	out, send, err := h.providers.DispatchMCP(rctx, segID, body.Token, body.RPC)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	if !send {
		ctx.Status(http.StatusNoContent)
		ctx.Writer.WriteHeaderNow()
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"rpc": json.RawMessage(out)})
}
