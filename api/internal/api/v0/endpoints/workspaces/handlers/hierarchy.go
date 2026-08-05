package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// errBadRequest is the internal signal that a helper has already written the
// response and the handler must stop. It never reaches a client.
var errBadRequest = errors.New("workspaces: request rejected")

// mergeRequest is the POST .../workspaces/:wsId/merge-into-parent body: the
// merge strategy to apply when folding the child branch into its parent, and
// whether to delete the now-merged child workspace once the merge succeeds.
type mergeRequest struct {
	Strategy     gitdomain.MergeStrategy `json:"strategy"`
	DeleteSource bool                    `json:"deleteSource"`
}

// reparentRequest is the POST .../workspaces/:wsId/reparent body: the id of the
// new parent the leaf child is rebased onto.
type reparentRequest struct {
	NewParentID string `json:"newParentId"`
}

// patchRequest is the PATCH .../workspaces/:wsId body. Branch renames the
// workspace's branch; FolderID and Order are its SIDEBAR placement — which
// folder it is filed under and where it sits among its siblings. The two travel
// on one endpoint because they are the two things a user edits about a row in
// place, and a nil field is left as it is.
//
// FolderID is never a fork parent. Re-parenting a fork is a git operation with
// its own endpoint (POST .../reparent) because it rebases; filing a row into a
// folder moves nothing on disk.
type patchRequest struct {
	Branch   *string `json:"branch"`
	FolderID *string `json:"folderId"`
	Order    *int    `json:"order"`
}

// Patch handles PATCH /v0/projects/:projectId/repos/:repoId/workspaces/:wsId. It
// renames the workspace's branch (relocating its workspace root to match) and/or
// files it into a folder at a given position.
//
// Unlike the other hierarchy mutations this answers SYNCHRONOUSLY. A rename is
// one directory rename rather than a long-running git operation and a placement
// is one row write, and every way either can be refused (the name is taken, the
// destination is occupied, the workspace is locked or adopted, the move would
// split a fork chain) is something the user has to see while the inline editor or
// the drag is still in front of them — a 202 would strand those behind a
// LastError frame. The updated workspace still arrives on the WebSocket stream,
// so callers do not patch their own cache.
func (h *Handlers) Patch(
	c *gin.Context,
) {
	var body patchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	id := c.Param("wsId")
	if err := h.applyRename(c, id, body.Branch); err != nil {
		return
	}
	if body.FolderID == nil && body.Order == nil {
		libs.WriteMutationOK(c, http.StatusOK, id)
		return
	}
	if h.placer == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "workspace placement unavailable")
		return
	}
	_, shifted, err := h.placer.PlaceWorkspace(
		c.Request.Context(),
		c.Param("projectId"),
		c.Param("repoId"),
		id,
		folder.PlaceInput{FolderID: body.FolderID, Order: body.Order},
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcastFolders(shifted)
	libs.WriteMutationOK(c, http.StatusOK, id)
}

// applyRename runs the branch half of the PATCH, writing the error response and
// returning non-nil when the caller must stop. A nil branch is a no-op.
func (h *Handlers) applyRename(
	c *gin.Context,
	id string,
	branch *string,
) error {
	if branch == nil {
		return nil
	}
	if strings.TrimSpace(*branch) == "" {
		libs.WriteErr(c, http.StatusBadRequest, "branch is required")
		return errBadRequest
	}
	if _, err := h.hierarchy.RenameBranch(c.Request.Context(), id, *branch); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return err
	}
	return nil
}

// broadcastFolders delivers the folder rows a placement renumbered. The
// workspace rows it touched need no such handling — every one is an aggregate
// write, and the hub projection broadcasts each on the way through — but folders
// are a plain GORM row whose only fan-out is this call.
func (h *Handlers) broadcastFolders(
	rows []domain.Folder,
) {
	for _, row := range rows {
		h.broadcastFolder(dto.FolderDTOFrom(row))
	}
}

// MergeIntoParent handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/merge-into-parent.
// It validates synchronously (body shape, strategy present, workspace exists)
// returning 4xx on failure; then it returns 202 and runs the local child→parent
// merge in the background. The merge outcome is delivered on the workspace
// WebSocket stream via the repository's broadcast callback; a failure surfaces as
// LastError on the entity (00 §4).
func (h *Handlers) MergeIntoParent(
	c *gin.Context,
) {
	var body mergeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Strategy == "" {
		libs.WriteErr(c, http.StatusBadRequest, "strategy is required")
		return
	}
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			result, mergeErr := h.hierarchy.MergeIntoParent(ctx, id, body.Strategy)
			if mergeErr != nil {
				return mergeErr
			}
			// Fold the now-merged child away only on a clean merge AND only when
			// it is a leaf: a conflict keeps it for the user to resolve, and a
			// non-leaf child is kept because cascade-deleting it would destroy
			// its descendants' unmerged work (no silent data loss either way).
			if !body.DeleteSource || result.ConflictsPending {
				return nil
			}
			return h.foldMergedChildIfLeaf(ctx, id)
		},
	)
}

// foldMergedChildIfLeaf removes the just-merged workspace when it is a leaf,
// leaving a non-leaf child in place because cascade-deleting it would destroy
// its descendants' unmerged work. It returns nil (a no-op) when the child still
// has children.
func (h *Handlers) foldMergedChildIfLeaf(
	ctx context.Context,
	id string,
) error {
	leaf, err := h.workspaceIsLeaf(ctx, id)
	if err != nil {
		return fmt.Errorf("merge succeeded but post-merge cleanup failed: %w", err)
	}
	if !leaf {
		return nil
	}
	if err := h.hierarchy.DeleteCascade(ctx, id); err != nil {
		return fmt.Errorf("merge succeeded but removing the workspace failed: %w", err)
	}
	return nil
}

// workspaceIsLeaf reports whether the workspace has no child workspaces, so it
// can be safely removed after a merge without cascade-deleting the unmerged work
// of any descendants.
func (h *Handlers) workspaceIsLeaf(ctx context.Context, id string) (bool, error) {
	all, err := h.reader.List(ctx)
	if err != nil {
		return false, err
	}
	for _, ws := range all {
		if ws.ParentID == id {
			return false, nil
		}
	}
	return true, nil
}

// Reparent handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/reparent. It
// validates synchronously (body shape, newParentId present, workspace exists)
// returning 4xx on failure; then it returns 202 and rebases the leaf child onto
// the new parent in the background. The reparented workspace is delivered on the
// workspace WebSocket stream via the repository's broadcast callback; a failure
// surfaces as LastError on the entity (00 §4).
// RebaseOntoParent handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/rebase-onto-parent.
// It is the user-initiated "finish the move" for a moved-but-conflicting child:
// 202 + async rebase of the child onto its current parent, keeping a conflicting
// rebase live for the standard resolve flow. The outcome rides the workspace WS
// stream; a failure surfaces as LastError on the entity.
func (h *Handlers) RebaseOntoParent(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, rebaseErr := h.hierarchy.RebaseOntoParent(ctx, id)
			return rebaseErr
		},
	)
}

func (h *Handlers) Reparent(
	c *gin.Context,
) {
	var body reparentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.NewParentID == "" {
		libs.WriteErr(c, http.StatusBadRequest, "newParentId is required")
		return
	}
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, reparentErr := h.hierarchy.Reparent(ctx, id, body.NewParentID)
			return reparentErr
		},
	)
}

// RetryProvision handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/retry-provision.
// It validates the workspace exists synchronously (4xx if not), then returns 202
// and re-provisions the placeholder in place in the background. The provisioned
// workspace is delivered on the workspace WebSocket stream; a failure (e.g. the
// branch is still held) surfaces as LastError on the entity.
func (h *Handlers) RetryProvision(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, retryErr := h.hierarchy.RetryProvision(ctx, id)
			return retryErr
		},
	)
}

// DetachHolder handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/detach-holder.
// It validates the workspace exists synchronously (4xx if not), then returns 202
// and, in the background, detaches the branch's holder (with the user's consent,
// captured by the modal that fires this call), clears the home row's branch when
// the holder is the repo home, and re-provisions the placeholder in place. A
// failure (e.g. detach blocked mid-merge) surfaces as LastError on the entity.
func (h *Handlers) DetachHolder(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, detachErr := h.hierarchy.DetachHolder(ctx, id)
			return detachErr
		},
	)
}
