package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

// Messages serves Crowbar's bounded, hook-derived chat ledger. Provider
// transcripts and raw hook payloads are not exposed.
func (h *Handlers) Messages(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	after, ok := intQuery(ctx, "after")
	if !ok {
		return
	}
	before, ok := intQuery(ctx, "before")
	if !ok {
		return
	}
	limit, ok := intQuery(ctx, "limit")
	if !ok {
		return
	}

	page, err := h.usecase.ReadMessages(ctx.Request.Context(), chat.ID, after, before, limit)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	items := make([]dto.AgentMessageDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, dto.AgentMessageDTO{
			Sequence:   item.Sequence,
			Role:       item.Role,
			ProviderID: item.Provider,
			Text:       item.Text,
			At:         item.At,
		})
	}
	libs.WriteQueryOK(ctx, dto.AgentMessagePageDTO{
		Cursor:       page.Cursor,
		OldestCursor: page.OldestCursor,
		HasMore:      page.HasMore,
		Items:        items,
	})
}

// SubmitPrompt synchronously returns once the replacement interactive TUI is
// live. Its hooks, not this handler, eventually append the user and assistant
// messages.
func (h *Handlers) SubmitPrompt(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		Text            string `json:"text"`
		ClientRequestID string `json:"clientRequestId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.usecase.SubmitPrompt(
		ctx.Request.Context(), chat.ID, body.Text, body.ClientRequestID,
	)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		if code := agentusecase.PromptErrorCode(err); code != "" {
			libs.WriteErrCode(ctx, status, code, message)
		} else {
			libs.WriteErr(ctx, status, message)
		}
		return
	}
	libs.WriteQueryOK(ctx, result)
}

// SlashCatalog executes the live provider's deterministic, descriptor-mapped
// probe for this one request. The response contains no raw command output and is
// not cached or persisted by the backend.
func (h *Handlers) SlashCatalog(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	catalog, err := h.usecase.SlashCatalog(ctx.Request.Context(), chat.ID)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		if code := agentusecase.CatalogErrorCode(err); code != "" {
			libs.WriteErrCode(ctx, status, code, message)
		} else {
			libs.WriteErr(ctx, status, message)
		}
		return
	}
	items := make([]dto.SlashCatalogItemDTO, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		items = append(items, dto.SlashCatalogItemDTO{
			ID:          item.ID,
			Kind:        item.Kind,
			Label:       item.Label,
			Description: item.Description,
			InsertText:  item.InsertText,
			Source:      item.Source,
		})
	}
	warnings := append([]string{}, catalog.Warnings...)
	libs.WriteQueryOK(ctx, dto.SlashCatalogDTO{
		ProviderID:   catalog.ProviderID,
		Completeness: string(catalog.Completeness),
		Items:        items,
		Warnings:     warnings,
	})
}

func intQuery(ctx *gin.Context, name string) (int, bool) {
	raw := ctx.Query(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, name+" must be an integer")
		return 0, false
	}
	return value, true
}
