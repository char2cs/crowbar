package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newUUID is the default id factory for new threads and messages.
func newUUID() string {
	return uuid.NewString()
}

// scope reads the hierarchical project/repo/workspace ids from the request path.
func scope(
	ctx *gin.Context,
) (string, string, string) {
	return ctx.Param("projectId"), ctx.Param("repoId"), ctx.Param("wsId")
}

// List handles GET /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads, returning the
// workspace's review threads as scoped DTOs.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	projectID, repoID, wsID := scope(ctx)
	threads, err := h.store.ListByWorkspace(ctx.Request.Context(), wsID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.ThreadDTOList(threads, projectID, repoID))
}

// OpenThread handles POST .../workspaces/:wsId/threads, opening a thread anchored to a file
// location. It returns 201 with the scoped DTO and broadcasts the same DTO.
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
		libs.Fail(ctx, err)
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, d)
}

// Detail handles GET .../workspaces/:wsId/threads/:threadId, returning the thread as a DTO with
// its anchor and ordered replies.
func (h *Handlers) Detail(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	thread, err := h.store.Get(ctx.Request.Context(), ctx.Param("threadId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID))
}

// SetResolved handles PATCH .../threads/:threadId, resolving or reopening a thread; it returns
// 200 with the scoped DTO and broadcasts it.
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
		libs.Fail(ctx, err)
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
	// No provider or chat on this user-facing route: agents reply through MCP, which attributes
	// off the authenticated caller rather than a request body anyone could type.
	thread, err := h.store.Reply(ctx.Request.Context(), reviewthread.ReplyInput{
		ID:        threadID,
		MessageID: h.newID(),
		Author:    body.Author,
		IsAgent:   body.IsAgent,
		Body:      body.Body,
	}, h.now())
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// EditMessage handles PATCH .../threads/:threadId/messages/:messageId, rewriting any message
// body including the root comment; it returns 200 with the scoped DTO and broadcasts it.
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
		libs.Fail(ctx, err)
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// DeleteMessage handles DELETE .../threads/:threadId/messages/:messageId, removing one reply
// (the root is refused as unprocessable; delete the thread instead). It returns 200 with the
// updated DTO and broadcasts it.
func (h *Handlers) DeleteMessage(
	ctx *gin.Context,
) {
	projectID, repoID, _ := scope(ctx)
	threadID := ctx.Param("threadId")
	messageID := ctx.Param("messageId")
	thread, err := h.store.DeleteMessage(ctx.Request.Context(), threadID, messageID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	d := dto.ThreadDTOFrom(thread.NormalizedMessages(), projectID, repoID)
	h.push(d)
	libs.WriteQueryOK(ctx, d)
}

// DeleteThread handles DELETE .../threads/:threadId, forgetting the whole thread. It broadcasts
// and returns a scoped tombstone DTO (Deleted=true) so subscribed clients drop it.
func (h *Handlers) DeleteThread(
	ctx *gin.Context,
) {
	projectID, repoID, wsID := scope(ctx)
	threadID := ctx.Param("threadId")
	if err := h.store.DeleteThread(ctx.Request.Context(), threadID); err != nil {
		libs.Fail(ctx, err)
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
