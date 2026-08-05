// Package handlers holds the gin handlers backing the folders endpoint.
package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Usecase is the folder surface the handlers need: CRUD plus the repo-scoped
// list the sidebar reads.
type Usecase interface {
	Create(
		ctx context.Context,
		in folder.CreateInput,
	) (domain.Folder, []domain.Folder, error)
	Rename(
		ctx context.Context,
		id string,
		name string,
	) (domain.Folder, error)
	Move(
		ctx context.Context,
		id string,
		in folder.MoveInput,
	) (domain.Folder, []domain.Folder, error)
	Delete(
		ctx context.Context,
		id string,
	) ([]domain.Folder, error)
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Folder, error)
}

// Handlers serves the /v0/projects/:projectId/repos/:repoId/folders routes.
//
// Every mutation here answers SYNCHRONOUSLY, unlike the workspace and repo
// mutations beside it. Those are fire-and-forget because their real work is a
// git operation measured in seconds; a folder write is one row, and every way it
// can be refused (a cycle, a cross-repo parent, a blank name) is something the
// user has to see while the drag or the inline editor is still in front of them.
// A 202 would strand those behind a frame that never comes.
//
// The updated entity still rides the Folders WebSocket stream, so callers never
// patch their own cache from the response.
type Handlers struct {
	usecase   Usecase
	broadcast func(dto.FolderDTO)
}

// New builds the folders Handlers. A nil broadcast degrades to a no-op so the
// handler never panics when wired without a hub (tests).
func New(
	usecase Usecase,
	broadcast func(dto.FolderDTO),
) *Handlers {
	if broadcast == nil {
		broadcast = func(dto.FolderDTO) {}
	}
	return &Handlers{usecase: usecase, broadcast: broadcast}
}

// createRequest is the POST .../folders body.
type createRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}

// patchRequest is the PATCH .../folders/:folderId body. Every field is optional
// and a nil field is left as it is, so a rename, a re-parent and a reorder are
// the same endpoint — which is what a drag that does two of them at once needs.
type patchRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// List handles GET /v0/projects/:projectId/repos/:repoId/folders, returning the
// repo's folders as FolderDTO[] in sidebar order.
func (h *Handlers) List(
	c *gin.Context,
) {
	rows, err := h.usecase.ListInRepo(c.Request.Context(), c.Param("projectId"), c.Param("repoId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.FolderDTOList(rows))
}

// Create handles POST /v0/projects/:projectId/repos/:repoId/folders. The URL
// scope is authoritative: a body-supplied project or repo would let a POST to
// one repo create a folder in another, so neither is accepted.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	created, shifted, err := h.usecase.Create(c.Request.Context(), folder.CreateInput{
		ProjectID: c.Param("projectId"),
		RepoID:    c.Param("repoId"),
		ParentID:  body.ParentID,
		Name:      body.Name,
	})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcastAll(append(shifted, created))
	libs.WriteMutationOK(c, http.StatusCreated, created.ID)
}

// Patch handles PATCH /v0/projects/:projectId/repos/:repoId/folders/:folderId:
// rename, re-parent and reorder, in that order, so a single drag that renames
// and moves lands as one broadcast rather than two half-states.
func (h *Handlers) Patch(
	c *gin.Context,
) {
	var body patchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	id := c.Param("folderId")
	updated, shifted, err := h.apply(c.Request.Context(), id, body)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcastAll(append(shifted, updated))
	libs.WriteMutationOK(c, http.StatusOK, id)
}

// apply runs the rename first and the placement second, so a drag that does
// both lands as one broadcast rather than two half-states. A PATCH that carries
// neither still goes through Move, which is a no-op placement whose only job
// here is to return the row's real state for the broadcast.
func (h *Handlers) apply(
	ctx context.Context,
	id string,
	body patchRequest,
) (domain.Folder, []domain.Folder, error) {
	if body.Name != nil {
		if _, err := h.usecase.Rename(ctx, id, *body.Name); err != nil {
			return domain.Folder{}, nil, err
		}
	}
	return h.usecase.Move(ctx, id, folder.MoveInput{ParentID: body.ParentID, Order: body.Order})
}

// broadcastAll fans every row a mutation wrote out on the Folders stream. The
// collateral matters as much as the subject: a move renumbers the levels it left
// and joined, and a client told only about the row that was dragged would hold
// stale orders for its siblings until the next reconnect.
func (h *Handlers) broadcastAll(
	rows []domain.Folder,
) {
	for _, row := range rows {
		h.broadcast(dto.FolderDTOFrom(row))
	}
}

// Delete handles DELETE /v0/projects/:projectId/repos/:repoId/folders/:folderId.
// What the folder held is REPARENTED to the folder's own parent, never deleted:
// a folder holds no worktrees, so removing the workspaces under it would destroy
// work the user only meant to unfile. A deleted-status FolderDTO tombstone rides
// the Folders stream so the client cache drops the entity (00 §6).
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("folderId")
	projectID := c.Param("projectId")
	repoID := c.Param("repoId")
	reparented, err := h.usecase.Delete(c.Request.Context(), id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcastAll(reparented)
	h.broadcast(dto.FolderDTO{ID: id, ProjectID: projectID, RepoID: repoID, Status: "deleted"})
	c.Status(http.StatusNoContent)
}
