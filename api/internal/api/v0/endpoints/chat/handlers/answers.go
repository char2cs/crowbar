package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
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
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, ctx.Param("choiceId"))
}

func (h *Handlers) AwaitHookAnswer(ctx *gin.Context) {
	var body struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.DeliveryID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "delivery_id is required")
		return
	}

	answer, err := h.answers.AwaitAnswer(ctx.Request.Context(), body.DeliveryID)
	if err != nil {
		if errors.Is(err, ctx.Request.Context().Err()) {
			libs.WriteQueryOK(ctx, dto.AgentHookAnswerDTO{})
			return
		}
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteQueryOK(ctx, dto.AgentHookAnswerDTO{Stdout: string(answer.Stdout)})
}

func (h *Handlers) AbandonHookAnswer(ctx *gin.Context) {
	var body struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.DeliveryID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "delivery_id is required")
		return
	}
	if err := h.answers.AbandonAnswer(ctx.Request.Context(), body.DeliveryID); err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteAccepted(ctx)
}
