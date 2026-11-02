// Package repos mounts the v0 repository REST routes.
package repos

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
)

// Register mounts the repo list and detail routes on the supplied router group,
// backed by the repository GORM store, which publishes every row it writes.
func Register(
	rg *gin.RouterGroup,
	store repohandlers.Store,
	prov repohandlers.BranchProviderEngine,
	wsReader repohandlers.WorkspaceReader,
	importer repohandlers.RepoImporter,
	updater repohandlers.RepoUpdater,
	placements *dto.Placements,
	remote repohandlers.RemoteRefresher,
	deleter repohandlers.RepoDeleter,
	ops repohandlers.Operations,
	crowbarHome func() (string, error),
) {
	h := repohandlers.NewWithDeps(store, prov, wsReader, ops, crowbarHome, placements).
		WithImporter(importer).
		WithUpdater(updater).
		WithRemoteRefresher(remote).
		WithRepoDeleter(deleter)
	rg.POST("/repos", h.Create)
	rg.GET("/repos", h.List)
	rg.GET("/repos/:repoId", h.Detail)
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
