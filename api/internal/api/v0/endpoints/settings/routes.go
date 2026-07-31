// Package settings registers the daemon-hosted client UI-state routes.
package settings

import (
	"github.com/gin-gonic/gin"

	settingshandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/settings/handlers"
)

// Register mounts the GET/PUT /settings/ui pair on settingsRG, the top-level
// /v0 group.
//
// It mounts OUTSIDE the entity hierarchy, beside /settings/terminal/profiles
// and /settings/agent/providers, for the same reason those two do: the value is
// addressed by an explicit ?scope= key, not by a path position. Nesting the
// per-workspace scope under /projects/:projectId/repos/:repoId/workspaces/:wsId
// would fork the surface into three routes with three shapes for one key-value
// store, and would make the machine-wide scope the odd one out.
//
// Scoping therefore rides in the query string, in three forms — "global",
// "repo:<repoId>", "workspace:<workspaceId>" — which is exactly how the client
// stores this replaces are keyed today: two are single-row global blobs, the
// workspace hierarchy is keyed by REPO id despite its name, and only the pane
// layout is keyed by workspace id.
//
// The value is opaque JSON. The daemon stores and returns it and knows nothing
// else about it, so the client can change its UI-state shape without a daemon
// change — which is the whole reason this pair exists rather than four typed
// endpoints.
func Register(
	settingsRG *gin.RouterGroup,
	store settingshandlers.UISettingsStore,
) {
	h := settingshandlers.New(store)

	settingsRG.GET("/settings/ui", h.GetUI)
	settingsRG.PUT("/settings/ui", h.PutUI)
}
