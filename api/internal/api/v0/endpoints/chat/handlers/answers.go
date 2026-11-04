package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

const maxAnswerReasonBytes = 4 << 10

const maxAnswerContentBytes = 64 << 10

func (h *Handlers) AnswerChoice(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}

	var body struct {
		OptionIDs []string `json:"optionIds"`

		Reason string `json:"reason"`

		Content json.RawMessage `json:"content"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.OptionIDs) == 0 {
		libs.WriteErr(ctx, http.StatusBadRequest, "an answer must name at least one option")
		return
	}
	if len(body.Reason) > maxAnswerReasonBytes {
		libs.WriteErr(ctx, http.StatusBadRequest, "reason is too long")
		return
	}
	if len(body.Content) > maxAnswerContentBytes {
		libs.WriteErr(ctx, http.StatusBadRequest, "content is too long")
		return
	}

	err := h.answers.AnswerChoice(
		ctx.Request.Context(), chat.ID, ctx.Param("choiceId"),
		body.OptionIDs, body.Reason, body.Content,
	)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, ctx.Param("choiceId"))
}

// AnswerTerminalWait types one of the descriptor-declared replies into the
// provider's own terminal prompt, so the human need not open the terminal.
func (h *Handlers) AnswerTerminalWait(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		AnswerID string `json:"answerId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil || body.AnswerID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "answerId is required")
		return
	}
	if err := h.answers.AnswerTerminalWait(ctx.Request.Context(), chat.ID, body.AnswerID); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, chat.ID)
}
