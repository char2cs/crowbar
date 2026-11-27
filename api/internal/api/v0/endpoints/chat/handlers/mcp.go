package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func (h *Handlers) MCP(
	ctx *gin.Context,
) {
	h.serveMCP(ctx, "segid", h.providers.DispatchMCP)
}

func (h *Handlers) RelayMCP(
	ctx *gin.Context,
) {
	h.serveMCP(ctx, "relay", h.providers.DispatchRelayMCP)
}

func (h *Handlers) serveMCP(
	ctx *gin.Context,
	param string,
	dispatch func(ctx context.Context, id, token string, message []byte) ([]byte, bool, error),
) {
	rctx := ctx.Request.Context()

	var body struct {
		Token string          `json:"token"`
		RPC   json.RawMessage `json:"rpc"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.RPC) == 0 {
		libs.WriteErr(ctx, http.StatusBadRequest, "body must carry an rpc message")
		return
	}

	out, send, err := dispatch(rctx, ctx.Param(param), body.Token, body.RPC)
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
