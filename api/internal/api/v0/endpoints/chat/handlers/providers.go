package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// UploadDescriptor installs a validated YAML descriptor as a new provider.
func (h *Handlers) UploadDescriptor(ctx *gin.Context) {
	const maxBytes = 1 << 20
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxBytes)
	raw, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			libs.WriteErr(ctx, http.StatusRequestEntityTooLarge, "Descriptor must be under 1 MB")
			return
		}
		libs.WriteErr(ctx, http.StatusBadRequest, "Descriptor must be a YAML file under 1 MB")
		return
	}
	report, err := h.providers.InstallDescriptor(ctx.Request.Context(), raw)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, report)
}

// Providers handles GET .../chats/providers and the global settings GET: the providers with connected
// and enabled flags, in priority order. The list is scope-independent, so every mount returns the same.
func (h *Handlers) Providers(
	ctx *gin.Context,
) {
	providers, err := h.providers.ResolveProviders(ctx.Request.Context())
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, providerDTOs(providers))
}

// DescriptorReports handles GET /v0/settings/chat/descriptors: every
// descriptor's static findings (rule, severity, YAML path and line, hint).
func (h *Handlers) DescriptorReports(
	ctx *gin.Context,
) {
	reports, err := h.providers.DescriptorReports(ctx.Request.Context())
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, reports)
}

// UpdateProviderPreferences handles PUT /v0/settings/chat/providers: the complete ordered list
// replaces the preference table (position is priority) and the resolved list comes back.
func (h *Handlers) UpdateProviderPreferences(
	ctx *gin.Context,
) {
	// Both flags arrive negated, so a missing field decodes to the absent-row default. On this full-replace
	// PUT an omitted mcpDisabled therefore re-enables the tool surface: clients must send both flags for
	// every provider on every write.
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
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, providerDTOs(resolved))
}

func providerDTOs(in []domain.AgentProvider) []dto.AgentProviderDTO {
	out := make([]dto.AgentProviderDTO, 0, len(in))
	for _, p := range in {
		out = append(out, dto.AgentProviderDTO{
			ID:                p.ID,
			DisplayName:       p.DisplayName,
			Icon:              p.Icon,
			SignIn:            p.SignIn,
			Connected:         p.Connected,
			Enabled:           p.Enabled,
			MCPEnabled:        p.MCPEnabled,
			Compaction:        p.Compaction,
			Hotswap:           p.Hotswap,
			PromptSteer:       p.PromptSteer,
			HasTerminal:       p.HasTerminal,
			TerminalStartHere: p.TerminalStartHere,
			ModelSelect:       p.ModelSelect,
			EffortSelect:      p.EffortSelect,
			ActivityEvents:    p.ActivityEvents,
			Models:            p.Models,
			DefaultModel:      p.DefaultModel,
			Efforts:           p.Efforts,
			DefaultEfforts:    p.DefaultEfforts,
			PermissionLevels:  p.PermissionLevels,
		})
	}
	return out
}
