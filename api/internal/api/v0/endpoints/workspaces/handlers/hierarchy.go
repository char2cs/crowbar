package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

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

// renameRequest is the PATCH .../workspaces/:wsId body: the branch name the
// workspace is renamed to.
type renameRequest struct {
	Branch string `json:"branch"`
}

// Rename handles PATCH /v0/projects/:projectId/repos/:repoId/workspaces/:wsId.
// It renames the workspace's branch and relocates its workspace root to match.
//
// Unlike the other hierarchy mutations this answers SYNCHRONOUSLY. A rename is
// one directory rename rather than a long-running git operation, and every way
// it can be refused (the name is taken, the destination is occupied, the
// workspace is locked or adopted) is something the user has to see while the
// inline editor is still in front of them — a 202 would strand those behind a
// LastError frame. The updated workspace still arrives on the WebSocket stream,
// so callers do not patch their own cache.
func (h *Handlers) Rename(
	c *gin.Context,
) {
	var body renameRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Branch) == "" {
		libs.WriteErr(c, http.StatusBadRequest, "branch is required")
		return
	}
	id := c.Param("wsId")
	renamed, err := h.hierarchy.RenameBranch(c.Request.Context(), id, body.Branch)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, renamed.ID)
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
