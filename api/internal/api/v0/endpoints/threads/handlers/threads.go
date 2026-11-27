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

func newUUID() string {
	return uuid.NewString()
}

func scope(
	ctx *gin.Context,
) (string, string, string) {
	return ctx.Param("projectId"), ctx.Param("repoId"), ctx.Param("wsId")
}

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

func (h *Handlers) push(
	d dto.ThreadDTO,
) {
	if h.broadcast == nil {
		return
	}
	h.broadcast.Push(d)
}
