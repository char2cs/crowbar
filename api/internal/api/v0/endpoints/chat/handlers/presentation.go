package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (h *Handlers) SubmitPrompt(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		Text            string `json:"text"`
		ClientRequestID string `json:"clientRequestId"`
		// RetryOf is the refused prompt this send retries; the ledger links the two.
		RetryOf string `json:"retryOf"`
		// Provider is the composer's staged provider — omitted or empty
		// means "nothing staged, use the chat's current one as-is." It has no
		// "clear it" reading at all: a chat always runs SOME provider.
		Provider string `json:"provider"`
		// Model/Effort are pointers because "" is a real pick (the provider default): nil leaves the sticky
		// selection alone. They bind as one pair, since valid efforts depend on the model.
		Model  *string `json:"model"`
		Effort *string `json:"effort"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	var selection *domain.ChatSelection
	if body.Model != nil || body.Effort != nil {
		selection = &domain.ChatSelection{
			Model:  derefOr(body.Model),
			Effort: derefOr(body.Effort),
		}
	}
	result, err := h.runners.SubmitPrompt(
		domain.WithPromptRetryOf(ctx.Request.Context(), body.RetryOf), chat.ID, body.Text, body.ClientRequestID,
		body.Provider, selection,
	)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.PromptSubmissionDTO{
		RunnerID:          result.RunnerID,
		TerminalSessionID: result.TerminalSessionID,
	})
}

func (h *Handlers) SlashCatalog(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	catalog, err := h.runners.SlashCatalog(ctx.Request.Context(), chat.ID)
	if err != nil {
		libs.Fail(ctx, err)
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
		Completeness: catalog.Completeness,
		Items:        items,
		Warnings:     warnings,
	})
}

// derefOr reads an optional wire string, treating absence as "".
func derefOr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
