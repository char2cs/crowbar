package v0

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// registerTerminalHandlers mounts all terminal REST routes on rg.
// If the engine container is nil (e.g., in legacy tests), the routes
// respond with 503.
func registerTerminalHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/terminals", c.handleCreateTerminal)
	rg.DELETE("/terminals/:sessionId", c.handleKillTerminal)

	rg.GET("/ws/terminals/:sessionId", c.handleTerminalWS)

	rg.GET("/settings/terminal/profiles", c.handleListProfiles)
	rg.GET("/settings/terminal/profiles/:id", c.handleGetProfile)
	rg.POST("/settings/terminal/profiles", c.handleCreateProfile)
	rg.PUT("/settings/terminal/profiles/:id", c.handleUpdateProfile)
	rg.DELETE("/settings/terminal/profiles/:id", c.handleDeleteProfile)
}

// handleCreateTerminal POST /v0/workspaces/:wsId/terminals
func (c *Container) handleCreateTerminal(
	ctx *gin.Context,
) {
	eng := c.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	wsID := ctx.Param("wsId")

	ws, err := c.app.Repositories.Workspace.Get(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	var body struct {
		ProfileID string `json:"profileId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	prof := c.resolveProfile(ctx.Request.Context(), body.ProfileID)

	sid, err := eng.Create(
		ctx.Request.Context(),
		wsID,
		ws.WorktreePath,
		prof,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"sessionId": sid})
}

// handleKillTerminal DELETE /v0/terminals/:sessionId
func (c *Container) handleKillTerminal(
	ctx *gin.Context,
) {
	eng := c.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	sid := ctx.Param("sessionId")
	if err := eng.Kill(ctx.Request.Context(), sid); err != nil {
		if errors.Is(err, engineterminal.ErrSessionNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// handleListProfiles GET /v0/settings/terminal/profiles
func (c *Container) handleListProfiles(
	ctx *gin.Context,
) {
	profiles, err := c.profileStore().FindAll(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, profiles)
}

// handleGetProfile GET /v0/settings/terminal/profiles/:id
func (c *Container) handleGetProfile(
	ctx *gin.Context,
) {
	id := ctx.Param("id")
	p, err := c.profileStore().FindByKey(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	ctx.JSON(http.StatusOK, p)
}

// handleCreateProfile POST /v0/settings/terminal/profiles
func (c *Container) handleCreateProfile(
	ctx *gin.Context,
) {
	var p domain.TerminalProfile
	if err := ctx.ShouldBindJSON(&p); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if p.ID == "" {
		p.ID = uuid.NewString()
	}

	if err := c.profileStore().Save(ctx.Request.Context(), p); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, p)
}

// handleUpdateProfile PUT /v0/settings/terminal/profiles/:id
func (c *Container) handleUpdateProfile(
	ctx *gin.Context,
) {
	id := ctx.Param("id")

	existing, err := c.profileStore().FindByKey(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	var p domain.TerminalProfile
	if err := ctx.ShouldBindJSON(&p); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p.ID = id

	if err := c.profileStore().Save(ctx.Request.Context(), p); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, p)
}

// handleDeleteProfile DELETE /v0/settings/terminal/profiles/:id
func (c *Container) handleDeleteProfile(
	ctx *gin.Context,
) {
	id := ctx.Param("id")

	existing, err := c.profileStore().FindByKey(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	if err := c.profileStore().Delete(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// requireTerminalEngine returns the engine or writes a 503 and returns nil.
func (c *Container) requireTerminalEngine(
	ctx *gin.Context,
) engineterminal.Engine {
	if c.eng == nil || c.eng.Terminal == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "terminal engine not available"})
		return nil
	}
	return c.eng.Terminal
}

// profileStore returns the TerminalProfile CRUD store.
func (c *Container) profileStore() store.Store[domain.TerminalProfile, string] {
	return c.app.GORM.TerminalProfiles
}

// resolveProfile fetches a profile by ID or returns nil if empty/missing.
func (c *Container) resolveProfile(
	ctx context.Context,
	profileID string,
) *domain.TerminalProfile {
	if profileID == "" {
		return nil
	}
	p, err := c.profileStore().FindByKey(ctx, profileID)
	if err != nil {
		return nil
	}
	return p
}
