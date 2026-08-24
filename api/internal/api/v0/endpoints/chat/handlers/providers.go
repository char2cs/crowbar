package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Providers handles GET .../workspaces/:wsId/chats/providers: the registered
// agent providers enriched with connected (installed) + enabled (!disabled) and
// returned in priority order (spec §3.1). The :wsId path param is retained for
// surface compatibility but the list is workspace-independent — the resolver reads
// crowbar home from app config, not the workspace — so every workspace yields the
// same ordered catalog.
func (h *Handlers) Providers(
	ctx *gin.Context,
) {
	providers, err := h.providers.ResolveProviders(ctx.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, providerDTOs(providers))
}

// UpdateProviderPreferences handles PUT /v0/settings/chat/providers: the full
// ordered preference set (spec §3.2). The body is the COMPLETE ordered list of
// known providers; the array position defines the new priority. The handler
// replaces the whole preference table (upsert submitted, delete omitted) and
// returns the freshly resolved list so the client reconciles from server truth
// with no second fetch. An unknown provider id is rejected 400 by the usecase
// (apperr.ErrInvalidArgument) before any row is written.
func (h *Handlers) UpdateProviderPreferences(
	ctx *gin.Context,
) {
	// Both flags arrive NEGATIVELY, matching what the row stores rather than what
	// the switch shows: a missing field decodes to false, which is the same
	// default an absent DB row carries.
	//
	// That is NOT the same as "an omitted field is harmless". This is a
	// full-replace PUT — the body is the complete preference set and every row is
	// rewritten from it — so a submitted provider whose mcpDisabled is omitted has
	// false WRITTEN over whatever the user chose, in the permissive direction: the
	// tool surface comes back on. Every client must therefore send both flags for
	// every provider on every write, whatever it is actually changing. The
	// frontend does exactly one thing about this and it is load-bearing: a single
	// commit() builds the whole flags table for every write path (see
	// providers-settings.tsx).
	var body struct {
		Providers []struct {
			ID          string `json:"id"`
			Disabled    bool   `json:"disabled"`
			MCPDisabled bool   `json:"mcpDisabled"`
		} `json:"providers"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	prefs := make([]domain.AgentProviderPreference, len(body.Providers))
	for i, p := range body.Providers {
		prefs[i] = domain.AgentProviderPreference{
			ProviderID:  p.ID,
			Priority:    i,
			Disabled:    p.Disabled,
			MCPDisabled: p.MCPDisabled,
		}
	}

	resolved, err := h.providers.ReplaceProviderPreferences(ctx.Request.Context(), prefs)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, providerDTOs(resolved))
}

func providerDTOs(in []domain.AgentProvider) []dto.AgentProviderDTO {
	out := make([]dto.AgentProviderDTO, 0, len(in))
	for _, p := range in {
		out = append(out, dto.AgentProviderDTO{
			ID:           p.ID,
			DisplayName:  p.DisplayName,
			Icon:         p.Icon,
			Connected:    p.Connected,
			Enabled:      p.Enabled,
			MCPEnabled:   p.MCPEnabled,
			Compaction:   p.Compaction,
			Hotswap:      p.Hotswap,
			HasTerminal:  p.HasTerminal,
			ModelSelect:  p.ModelSelect,
			EffortSelect: p.EffortSelect,
			Models:       p.Models,
			Efforts:      p.Efforts,
		})
	}
	return out
}
