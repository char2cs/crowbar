package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type HomeWorkspaces interface {
	GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)
}

type Files = filehandlers.Files

type ChatResolver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
}

type WorkSignal interface {
	WorkingFor(
		wsID string,
	) bool
}

type OwnerResolver interface {
	OwnerOf(
		ctx context.Context,
		ws domain.Workspace,
	) string
}

type Handlers struct {
	workspaces HomeWorkspaces
	files      Files
	working    WorkSignal
	chats      ChatResolver
	owners     OwnerResolver
}

func New(
	workspaces HomeWorkspaces,
	files Files,
	working WorkSignal,
) *Handlers {
	return &Handlers{
		workspaces: workspaces,
		files:      files,
		working:    working,
	}
}

func (h *Handlers) WithChats(
	chats ChatResolver,
) *Handlers {
	if chats != nil {
		h.chats = chats
	}
	return h
}

func (h *Handlers) WithOwners(
	owners OwnerResolver,
) *Handlers {
	if owners != nil {
		h.owners = owners
	}
	return h
}

func (h *Handlers) resolveHome(c *gin.Context) (domain.Workspace, bool) {
	ws, err := h.workspaces.GetHomeForProject(c.Request.Context(), c.Param("projectId"))
	if err == nil {
		return ws, true
	}
	if errors.Is(err, problem.ErrNotFound) {
		libs.WriteErr(c, http.StatusNotFound, "home workspace not found")
		return domain.Workspace{}, false
	}
	libs.WriteErr(c, http.StatusInternalServerError, "failed to resolve home workspace")
	return domain.Workspace{}, false
}

func (h *Handlers) RequireHomeWorkspace(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		c.Abort()
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "wsId", Value: ws.ID})
	c.Next()
}

func (h *Handlers) RequireHomeTerminalScope(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		c.Abort()
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "chatId", Value: ws.ID})
	reqscope.SetWorkspace(c, ws)
	c.Next()
}
