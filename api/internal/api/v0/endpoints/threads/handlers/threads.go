package handlers

import (
	"errors"
	"net/http"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newUUID is the default id factory for new threads and messages.
func newUUID() string {
	return uuid.NewString()
}

// threadErrorStatus maps a store error to an HTTP status: a not-found aggregate
// yields 404, a command validation failure (e.g. deleting the root via the
// message route, or editing a message that no longer exists) yields 400 as a
// deterministic client error, and every other failure yields 500 so a real
// store error is never masked.
func threadErrorStatus(
	err error,
) int {
	if errors.Is(err, apperr.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, asynxModels.ErrValidation) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// scope reads the hierarchical project/repo/workspace ids from the request path.
func scope(
	ctx *gin.Context,
) (string, string, string) {
	return ctx.Param("projectId"), ctx.Param("repoId"), ctx.Param("wsId")
}

// List handles GET
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads, returning the
// workspace's review threads as scoped DTOs. A WebSocket upgrade on the same
// path is routed to the thread broadcaster by the dual-serve wrapper.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	projectID, repoID, wsID := scope(ctx)
	threads, err := h.store.ListByWorkspace(ctx.Request.Context(), wsID)
	if err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, dto.ThreadDTOList(threads, projectID, repoID))
}

// OpenThread handles POST
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads, opening a new
// review thread anchored to a file location. It returns 201 with the scoped DTO
// and broadcasts the same DTO so the workspace-scoped stream converges.
func (h *Handlers) OpenThread(
	ctx *gin.Context,
) {
	projectID, repoID, wsID := scope(ctx)
	var body struct {
		FilePath  string            `json:"filePath"`
		Line      int               `json:"line"`
		StartLine int               `json:"startLine"`
		EndLine   int               `json:"endLine"`
		Side      domain.ReviewSide `json:"side"`
		Author    string            `json:"author"`
		IsAgent   bool              `json:"isAgent"`
		Body      string            `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	thread, err := h.store.Open(ctx.Request.Context(), reviewthread.OpenInput{
		ID:         h.newID(),
		WsID:       wsID,
		FilePath:   body.FilePath,
		LineNumber: body.Line,
		StartLine:  body.StartLine,
		EndLine:    body.EndLine,
		Side:       body.Side,
		MessageID:  h.newID(),
		Author:     body.Author,
		IsAgent:    body.IsAgent,
		Body:       body.Body,
	}, h.now())
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, d)
}

// Detail handles GET
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId,
// returning the single thread as a DTO with its anchor and ordered replies.
func (h *Handlers) Detail(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	thread, err := h.store.Get(ctx.Request.Context(), ctx.Param("threadId"))
	if err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID))
}

// SetResolved handles PATCH
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId,
// resolving or reopening a thread. It returns 200 with the scoped DTO and
// broadcasts it.
func (h *Handlers) SetResolved(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	threadID := ctx.Param("threadId")
	var body struct {
		IsResolved bool `json:"isResolved"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	thread, err := h.setResolved(ctx, threadID, body.IsResolved)
	if err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// setResolved dispatches to the resolve or reopen store method.
func (h *Handlers) setResolved(
	ctx *gin.Context,
	threadID string,
	resolved bool,
) (domain.ReviewThread, error) {
	if resolved {
		return h.store.Resolve(ctx.Request.Context(), threadID)
	}
	return h.store.Reopen(ctx.Request.Context(), threadID)
}

// Reply handles POST
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId/replies,
// appending a reply. It returns 200 with the scoped DTO and broadcasts it.
func (h *Handlers) Reply(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	threadID := ctx.Param("threadId")
	var body struct {
		Author  string `json:"author"`
		IsAgent bool   `json:"isAgent"`
		Body    string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	thread, err := h.store.Reply(ctx.Request.Context(), threadID, h.newID(), body.Author, body.IsAgent, body.Body, h.now())
	if err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// EditMessage handles PATCH
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId/messages/:messageId,
// rewriting a message body. It targets any message by id, including the root
// comment. Returns 200 with the scoped DTO and broadcasts it.
func (h *Handlers) EditMessage(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	threadID := ctx.Param("threadId")
	messageID := ctx.Param("messageId")
	var body struct {
		Body string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	thread, err := h.store.EditMessage(ctx.Request.Context(), threadID, messageID, body.Body)
	if err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// DeleteMessage handles DELETE
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId/messages/:messageId,
// removing a single reply. The root comment cannot be removed this way (deleting
// the root is DELETE on the thread); attempting it yields a 400. Returns 200 with
// the updated thread DTO and broadcasts it.
func (h *Handlers) DeleteMessage(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	threadID := ctx.Param("threadId")
	messageID := ctx.Param("messageId")
	thread, err := h.store.DeleteMessage(ctx.Request.Context(), threadID, messageID)
	if err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// DeleteThread handles DELETE
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId,
// forgetting the whole thread (root comment + replies). It broadcasts a tombstone
// DTO (Deleted=true) carrying the entity scope so subscribed clients drop it, and
// returns 200 with the same tombstone.
func (h *Handlers) DeleteThread(
	ctx *gin.Context,
) {
	projectID, repoID, wsID := scope(ctx)
	threadID := ctx.Param("threadId")
	if err := h.store.DeleteThread(ctx.Request.Context(), threadID); err != nil {
		libs.WriteErr(ctx, threadErrorStatus(err), err.Error())
		return
	}
	tombstone := dto.ThreadDTO{
		ID:          threadID,
		ProjectID:   projectID,
		RepoID:      repoID,
		WorkspaceID: wsID,
		Deleted:     true,
	}
	h.push(tombstone)
	libs.WriteQueryOK(ctx, tombstone)
}

// push forwards a thread DTO to the broadcaster when one is wired.
func (h *Handlers) push(
	d dto.ThreadDTO,
) {
	if h.broadcast == nil {
		return
	}
	h.broadcast.Push(d)
}
