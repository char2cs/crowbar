package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// errBadRequest is the internal signal that a helper has already written the
// response and the handler must stop. It never reaches a client.
var errBadRequest = errors.New("worktree: request rejected")

// mergeRequest is the POST .../chats/:id/merge-into-parent body: the merge
// strategy to apply when folding the child branch into its parent, and whether
// to delete the now-merged child workspace once the merge succeeds.
type mergeRequest struct {
	Strategy     gitdomain.MergeStrategy `json:"strategy"`
	DeleteSource bool                    `json:"deleteSource"`
}

// reparentRequest is the POST .../chats/:id/reparent body: the id of the new
// parent the leaf child is rebased onto.
type reparentRequest struct {
	NewParentID string `json:"newParentId"`
}

// applyRename runs the branch rename, writing the error response and returning
// non-nil when the caller must stop. A nil branch is a no-op.
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
		libs.Fail(c, err)
		return err
	}
	return nil
}

// ChatMergeIntoParent handles POST .../chats/:id/merge-into-parent. Validation failures are 4xx;
// then it answers 202 with the operation running the local child→parent merge and the leaf fold.
func (h *Handlers) ChatMergeIntoParent(
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
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	label := h.relLabel(c.Request.Context(), "Merge "+ws.Branch, "into", ws.ParentID)
	h.startOn(c, ws, domain.OpWorkspaceMergeIntoParent, label, func(ctx context.Context) error {
		result, mergeErr := h.hierarchy.MergeIntoParent(ctx, ws.ID, body.Strategy)
		if mergeErr != nil {
			return mergeErr
		}
		// Fold only a clean merge: a conflict keeps the child for the user to resolve.
		if !body.DeleteSource || result.ConflictsPending {
			return nil
		}
		return h.foldMergedChildIfLeaf(ctx, ws.ID)
	})
}

// foldMergedChildIfLeaf removes the just-merged workspace only when it is a leaf: cascade-deleting
// a non-leaf would destroy its descendants' unmerged work.
func (h *Handlers) foldMergedChildIfLeaf(
	ctx context.Context,
	id string,
) error {
	leaf, err := h.workspaceIsLeaf(ctx, id)
	if err != nil {
		return &problem.Error{Code: problem.Internal, Message: "merge succeeded but post-merge cleanup failed", Err: err}
	}
	if !leaf {
		return nil
	}
	// Never with consent: a fold nobody reviewed must not destroy unmerged work.
	if err := h.hierarchy.DeleteCascade(ctx, id, domain.KeepWorkAtRisk); err != nil {
		return &problem.Error{Code: problem.Internal, Message: "merge succeeded but removing the workspace failed", Err: err}
	}
	return nil
}

// workspaceIsLeaf reports whether the workspace has no child workspaces.
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

// ChatRebaseOntoParent handles POST .../chats/:id/rebase-onto-parent, the user's "finish the move"
// for a moved-but-conflicting child; a conflicting rebase stays live for the resolve flow.
func (h *Handlers) ChatRebaseOntoParent(
	c *gin.Context,
) {
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	label := h.relLabel(c.Request.Context(), "Rebase "+ws.Branch, "onto", ws.ParentID)
	h.startOn(c, ws, domain.OpWorkspaceRebaseOntoParent, label, func(ctx context.Context) error {
		_, err := h.hierarchy.RebaseOntoParent(ctx, ws.ID)
		return err
	})
}

// ChatReparent handles POST .../chats/:id/reparent: 4xx on validation, then 202 with the operation
// rebasing the leaf onto the new parent. newParentId names a workspace, not a chat: it is the git
// lineage parent the caller reads from the tree it draws.
func (h *Handlers) ChatReparent(
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
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	label := h.relLabel(c.Request.Context(), "Move "+ws.Branch, "under", body.NewParentID)
	h.startOn(c, ws, domain.OpWorkspaceReparent, label, func(ctx context.Context) error {
		_, err := h.hierarchy.Reparent(ctx, ws.ID, body.NewParentID)
		return err
	})
}

// ChatRetryProvision handles POST .../chats/:id/retry-provision, answering 202 with the operation
// re-provisioning the placeholder in place (it fails if the branch is still held).
func (h *Handlers) ChatRetryProvision(
	c *gin.Context,
) {
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	h.startOn(c, ws, domain.OpWorkspaceRetryProvision, "Retry "+ws.Branch, func(ctx context.Context) error {
		_, err := h.hierarchy.RetryProvision(ctx, ws.ID)
		return err
	})
}

// ChatDetachHolder handles POST .../chats/:id/detach-holder (the user consented in the modal that
// fires it), answering 202 with the operation that detaches the branch's holder and
// re-provisions the placeholder in place.
func (h *Handlers) ChatDetachHolder(
	c *gin.Context,
) {
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	h.startOn(c, ws, domain.OpWorkspaceDetachHolder, "Detach "+ws.HeldByPath, func(ctx context.Context) error {
		_, err := h.hierarchy.DetachHolder(ctx, ws.ID)
		return err
	})
}
