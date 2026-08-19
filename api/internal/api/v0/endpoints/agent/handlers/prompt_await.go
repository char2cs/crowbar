package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// AwaitPrompt handles POST .../workspaces/:wsId/agent/runners/:segid/prompt-await:
// a collector the vendor CLI itself started blocks here until the chat its runner
// is on has a message for it, and the user's own words go back in the reply.
//
// IT IS THE ONE CALLBACK IN THIS PACKAGE THAT READS RATHER THAN WRITES, which is
// why it is runner-keyed and token-authenticated exactly like MCP and unlike
// Hooks: the caller's whole scope is derived from (:segid, token) and never from
// the URL, and a segment id alone would be no credential at all — segment ids are
// published on the chats API.
//
// A poll with nothing to collect answers 204, and the collector asks again. That
// is not an error state, it is the steady state: a collector spends its entire
// life here and only leaves when there is something to carry.
//
// The acknowledgement is deliberate and it is taken AFTER the body is written.
// The daemon hands a message over exactly once, so "did this delivery complete"
// cannot be inferred later from anything durable — ack is the only record, and
// taking it before the write would record a delivery that had not happened yet.
func (h *Handlers) AwaitPrompt(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	segID := ctx.Param("segid")

	var body struct {
		Token  string `json:"token"`
		WaitMS int64  `json:"waitMs"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	prompt, found, ack, err := h.usecase.AwaitQueuedPrompt(rctx, segID, body.Token, body.WaitMS)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	if !found {
		ctx.Status(http.StatusNoContent)
		ctx.Writer.WriteHeaderNow()
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"prompt": prompt})
	ctx.Writer.Flush()
	ack()
}
