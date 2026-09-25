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
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
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

	// ReadContent reads a file in a workspace.
	ReadContent(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domain.FileContent, error)

	// WriteContent writes a file and resyncs the working tree. encoding is
	// "base64" for a byte-faithful binary payload or "" / "utf8" for raw UTF-8.
	WriteContent(
		ctx context.Context,
		wsID string,
		filePath string,
		content string,
		encoding string,
		now time.Time,
	) error

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

// ChatResolver resolves the chat rows a workspace id owns, mirroring the
// workspaces handlers' own ChatResolver (workspacehandlers.ChatResolver): the
// home workspace's GET is a second wire-DTO call site, and needs the same
// read to resolve WorkspaceDTO.OwningChatID.
type ChatResolver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
}

// WorkSignal is the read half of the daemon's working overlay, the same seam the
// workspaces handlers stamp their REST reads from (workspacehandlers.WorkSignal,
// which the repositories Container satisfies). The home workspace is served by
// THIS endpoint rather than the workspace-scoped list/detail, so it must stamp
// the overlay itself or its REST read would report working=false while an agent
// chat anchored to the project home is mid-turn — diverging from the very WS
// frames the container enriches for it. Only the read is needed here: the home
// endpoint runs no async mutations, so it never brackets a work window.
type WorkSignal interface {
	// WorkingFor reports whether the workspace is working via EITHER derived
	// overlay (an inflight background mutation OR an agent chat mid-turn).
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
	if errors.Is(err, apperr.ErrNotFound) {
		libs.WriteErr(c, http.StatusNotFound, "home workspace not found")
		return domain.Workspace{}, false
	}
	libs.WriteErr(c, http.StatusInternalServerError, "failed to resolve home workspace")
	return domain.Workspace{}, false
}

// RequireHomeWorkspace resolves the project's home workspace and injects its id
// as the :wsId path param, so handlers and the WS broadcaster reused from the
// repo-scoped surface (the file-change WS, review threads) resolve the home
// workspace by id without a dedicated home implementation. The home workspace
// has no repo, so :repoId stays empty — the thread namespace, filters, and
// snapshot all tolerate the empty middle segment (clientScope trims only
// trailing empties, so "p//w" still prefix-matches "p//w/<id>"). On failure
// resolveHome has already written the error envelope; we just abort the chain.
func (h *Handlers) RequireHomeWorkspace(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		c.Abort()
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "wsId", Value: ws.ID})
	c.Next()
}

// RequireHomeTerminalScope puts a home request in the shape the ONE terminal handler
// set expects: the session owner is the home workspace (it has no owning chat), so its
// id rides as :chatId, and the workspace itself is stashed where the chat-scoped
// resolveChatWorktree middleware would have put it — the PTY's starting directory.
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
