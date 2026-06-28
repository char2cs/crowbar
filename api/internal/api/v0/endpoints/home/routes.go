// Package home mounts the project-level home workspace routes under
// /v0/projects/:projectId/home. The home workspace has no git operations.
package home

import (
	"github.com/gin-gonic/gin"

	homehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
)

// Register mounts all home routes under projectScoped
// (/v0/projects/:projectId).
func Register(
	projectScoped *gin.RouterGroup,
	workspaces homehandlers.HomeWorkspaces,
	projects homehandlers.ProjectReader,
	files homehandlers.Files,
	termEng homehandlers.TerminalEngine,
) {
	h := homehandlers.New(workspaces, projects, files, termEng)
	home := projectScoped.Group("/home")

	home.GET("", h.Get)

	home.GET("/files/tree", h.FileTree)
	home.GET("/files/content", h.FileContent)
	home.PUT("/files/content", h.SaveFileContent)
	home.POST("/files", h.CreateFile)
	home.PATCH("/files", h.RenameFile)
	home.DELETE("/files", h.DeleteFile)

	home.GET("/terminals", h.ListTerminals)
	home.POST("/terminals", h.CreateTerminal)
	home.DELETE("/terminals/:sessionId", h.KillTerminal)
	home.GET("/terminals/:sessionId/ws", h.TerminalWS)
}
