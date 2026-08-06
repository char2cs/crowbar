// Package repos mounts the v0 repository REST routes.
package repos

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
)

// Register mounts the repo list and detail routes on the supplied router group,
// backed by the repository GORM store. The list and detail GET routes are
// dual-served: a plain GET answers REST while a WebSocket upgrade is routed to
// reposWS (the Broadcaster[RepoDTO] handle) for the live stream — a list-scope
// subscriber at /projects/:p/repos receives that project's repos, a
// :repoId-scope subscriber receives only that repo (W7-2).
func Register(
	rg *gin.RouterGroup,
	store repohandlers.Store,
	prov repohandlers.BranchProviderEngine,
	wsReader repohandlers.WorkspaceReader,
	importer repohandlers.RepoImporter,
	updater repohandlers.RepoUpdater,
	remote repohandlers.RemoteRefresher,
	wsRemover repohandlers.WorkspaceRemover,
	wsPurger repohandlers.WorkspacePurger,
	broadcast func(dto.RepoDTO),
	reposWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := repohandlers.NewWithDeps(store, prov, wsReader, broadcast).
		WithImporter(importer).
		WithUpdater(updater).
		WithRemoteRefresher(remote).
		WithWorkspaceRemover(wsRemover, wsPurger)
	rg.POST("/repos", h.Create)
	rg.GET("/repos", dispatch(h.List, reposWS))
	rg.GET("/repos/:repoId", dispatch(h.Detail, reposWS))
	rg.PATCH("/repos/:repoId", h.Patch)
	rg.DELETE("/repos/:repoId", h.DeleteRepo)
	rg.GET("/repos/:repoId/icon", h.Icon)
	rg.PUT("/repos/:repoId/icon", h.PutIcon)
	rg.DELETE("/repos/:repoId/icon", h.DeleteIcon)
	rg.PUT("/repos/:repoId/icon/emoji", h.PutIconEmoji)
	rg.PUT("/repos/:repoId/icon/github", h.PutIconGithub)
	rg.GET("/repos/:repoId/branches", h.Branches)
	rg.GET("/repos/:repoId/pull-requests", h.PullRequests)
}
