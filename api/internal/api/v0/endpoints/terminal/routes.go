package terminal

import (
	"github.com/gin-gonic/gin"

	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
) {
	h := termhandlers.New(termEng, profileStore)

	chatScoped.GET("/terminals", h.ListSessions)
	chatScoped.POST("/terminals", h.CreateSession)
	chatScoped.DELETE("/terminals/:sessionId", h.KillSession)
	chatScoped.GET("/terminals/:sessionId/ws", h.WS)

	settingsRG.GET("/settings/terminal/theme", h.HostTheme)
	settingsRG.PUT("/settings/terminal/theme", h.SetHostTheme)

	settingsRG.GET("/settings/terminal/profiles", h.ListProfiles)
	settingsRG.GET("/settings/terminal/profiles/:id", h.GetProfile)
	settingsRG.POST("/settings/terminal/profiles", h.CreateProfile)
	settingsRG.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	settingsRG.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
}
