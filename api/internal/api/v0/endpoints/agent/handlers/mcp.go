package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// MCP handles POST .../workspaces/:wsId/agent/runners/:segid/mcp: it carries one
// JSON-RPC message from a vendor CLI's MCP client to the daemon's tool surface,
// and the reply back. The body is {"token": string, "rpc": <raw JSON-RPC>}; the
// reply rides under the standard envelope as data.rpc, and a message the server
// answers with silence (a JSON-RPC notification) gets a bare 204 instead.
//
// The rpc message is forwarded RAW. JSON-RPC framing belongs to the engine, so
// the handler never parses or re-encodes it: a handler that reshaped a message
// it does not understand would corrupt a protocol revision it was not written
// for.
//
// Like RenameByRunner and Hooks it is runner-keyed and so makes NO
// requireChatInWorkspace check: the caller is authenticated by (:segid, token)
// and its entire scope is derived from the runner, never from the URL. The path
// ids exist only because every in-PTY callback builds a workspace-nested agent
// URL.
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

	out, send, err := h.usecase.DispatchMCP(rctx, segID, body.Token, body.RPC)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	if !send {
		ctx.Status(http.StatusNoContent)
		ctx.Writer.WriteHeaderNow()
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"rpc": json.RawMessage(out)})
}
