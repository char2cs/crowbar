// Package handlers serves the /v0/projects/:projectId/home routes.
package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// HomeWorkspaces is the workspace surface the home handlers need.
type HomeWorkspaces interface {
	GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)
}

// Files is the file usecase surface needed by home file handlers.
type Files interface {
	// Tree returns one level of the file tree for a workspace.
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider fileusecase.FileStatusProvider,
	) ([]domain.FileNode, error)

	// CreateFile creates a file and resyncs the working tree.
	CreateFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error

	// CreateDir creates a directory and resyncs the working tree.
	CreateDir(
		ctx context.Context,
		wsID string,
		dirPath string,
		now time.Time,
	) error

	// Copy duplicates a file or directory byte-faithfully and resyncs the
	// working tree.
	Copy(
		ctx context.Context,
		wsID string,
		sourcePath string,
		destPath string,
		now time.Time,
	) error

	// Rename renames a path and resyncs the working tree.
	Rename(
		ctx context.Context,
		wsID string,
		oldPath string,
		newPath string,
		now time.Time,
	) error

	// Delete removes a path and resyncs the working tree.
	Delete(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
}

// ChatResolver resolves the chat rows a workspace id owns, so GET /home can
// fill WorkspaceDTO.OwningChatID the same way the workspaces handlers do.
type ChatResolver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
}

// WorkSignal is whether an agent chat in a workspace is mid-turn. The home
// workspace's only REST read is here, so it stamps Working itself to agree
// with the WS frames the repositories Container enriches.
type WorkSignal interface {
	// WorkingFor reports whether an agent chat in the workspace is mid-turn.
	WorkingFor(
		wsID string,
	) bool
}

// OwnerResolver answers the chat that owns a workspace — the chat handlers'
// OwnerOf.
type OwnerResolver interface {
	OwnerOf(
		ctx context.Context,
		ws domain.Workspace,
	) string
}

// Handlers serves all /home/* routes.
type Handlers struct {
	workspaces HomeWorkspaces
	files      Files
	working    WorkSignal
	chats      ChatResolver
	owners     OwnerResolver
}

// New builds Handlers.
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

// WithChats wires the chat-row resolver Get needs to answer
// WorkspaceDTO.OwningChatID for the home workspace. A nil chats leaves it
// unwired, and Get degrades to an empty owningChatId rather than panicking.
func (h *Handlers) WithChats(
	chats ChatResolver,
) *Handlers {
	if chats != nil {
		h.chats = chats
	}
	return h
}

// WithOwners wires the owner resolver Get answers WorkspaceDTO.OwningChatID
// through.
func (h *Handlers) WithOwners(
	owners OwnerResolver,
) *Handlers {
	if owners != nil {
		h.owners = owners
	}
	return h
}

// resolveHome fetches the project's home workspace. Every project gets its home
// at creation (project import's createHomeWorkspace), so a missing one is a
// 404, never provisioned on a read.
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

// RequireHomeWorkspace injects the project's home workspace id as :wsId so
// handlers reused from the workspace-scoped surface resolve it by id. :repoId
// stays empty; thread scopes tolerate the empty middle segment ("p//w").
func (h *Handlers) RequireHomeWorkspace(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		c.Abort()
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "wsId", Value: ws.ID})
	c.Next()
}

// RequireHomeTerminalScope shapes a home request for the chat-scoped terminal handlers:
// the home workspace has no owning chat, so its id rides as :chatId, and the workspace
// is stashed in reqscope as resolveChatWorktree would, giving the PTY its directory.
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
