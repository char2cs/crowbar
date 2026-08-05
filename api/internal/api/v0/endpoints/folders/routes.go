// Package folders mounts the v0 folder REST routes and the dual-served list
// that upgrades to the live folders WebSocket stream on demand.
package folders

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	folderhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/folders/handlers"
)

// Register mounts the folder list, create, patch and delete routes on the
// repo-scoped router group. The list GET is dual-served: a plain GET answers
// REST while a WebSocket upgrade is routed to foldersWS (the
// Broadcaster[FolderDTO] handle) for the live stream, so one URL is both the
// snapshot and the subscription.
func Register(
	rg *gin.RouterGroup,
	usecase folderhandlers.Usecase,
	broadcast func(dto.FolderDTO),
	foldersWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := folderhandlers.New(usecase, broadcast)
	rg.GET("/folders", dispatch(h.List, foldersWS))
	rg.POST("/folders", h.Create)
	rg.PATCH("/folders/:folderId", h.Patch)
	rg.DELETE("/folders/:folderId", h.Delete)
}
