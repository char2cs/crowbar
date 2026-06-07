// Package provider mounts the v0 read-only provider REST routes: the
// per-workspace provider state (protected flag plus current pull request) and
// the per-repo protected-branch list (02 §2.6). The protected-branches route
// lives here, not on the repos endpoint, because both routes share the provider
// engine. All provider I/O is read-only.
package provider

import (
	"github.com/gin-gonic/gin"

	providerhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
)

// Register mounts the provider-state and protected-branches routes on the
// supplied router group, backed by the provider engine, the workspace reader,
// and the repo reader. The protected-branches route resolves its :id as a repo
// id against the repo reader, not a workspace id.
func Register(
	rg *gin.RouterGroup,
	providerEng providerhandlers.ProviderEngine,
	wsReader providerhandlers.WorkspaceReader,
	repos providerhandlers.RepoReader,
) {
	h := providerhandlers.New(providerEng, wsReader, repos)
	rg.GET("/workspaces/:wsId/provider", h.ProviderState)
	rg.GET("/repos/:id/protected-branches", h.ProtectedBranches)
}
