package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Providers handles GET .../workspaces/:wsId/agent/providers: the registered
// agent providers enriched with connected (installed) + enabled (!disabled) and
// returned in priority order (spec §3.1). The :wsId path param is retained for
// surface compatibility but the list is workspace-independent — the resolver reads
// crowbar home from app config, not the workspace — so every workspace yields the
// same ordered catalog.
func (h *Handlers) Providers(
	ctx *gin.Context,
) {
	providers, err := h.usecase.ResolveProviders(ctx.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, providers)
}

// UpdateProviderPreferences handles PUT /v0/settings/agent/providers: the full
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
	// the switch shows, so an omitted field means the default in the same
	// direction the DB does: a client that predates mcpDisabled submits nothing
	// for it and every provider keeps its tool surface.
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

	resolved, err := h.usecase.ReplaceProviderPreferences(ctx.Request.Context(), prefs)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, resolved)
}
