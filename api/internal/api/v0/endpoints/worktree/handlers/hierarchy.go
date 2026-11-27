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

var errBadRequest = errors.New("worktree: request rejected")

type mergeRequest struct {
	Strategy     gitdomain.MergeStrategy `json:"strategy"`
	DeleteSource bool                    `json:"deleteSource"`
}

type reparentRequest struct {
	NewParentID string `json:"newParentId"`
}

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
	if body.DeleteSource {
		if err := h.hierarchy.GuardMergeFold(c.Request.Context(), ws.ID); err != nil {
			libs.Fail(c, err)
			return
		}
	}
	label := h.relLabel(c.Request.Context(), "Merge "+ws.Branch, "into", ws.ParentID)
	h.startOn(c, ws, domain.OpWorkspaceMergeIntoParent, label, func(ctx context.Context) error {
		if body.DeleteSource {
			if err := h.hierarchy.GuardMergeFold(ctx, ws.ID); err != nil {
				return err
			}
		}
		result, mergeErr := h.hierarchy.MergeIntoParent(ctx, ws.ID, body.Strategy)
		if mergeErr != nil {
			return mergeErr
		}
		if !body.DeleteSource || result.ConflictsPending {
			return nil
		}
		return h.foldMergedChildIfLeaf(ctx, ws.ID)
	})
}

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
	if err := h.hierarchy.DeleteCascade(ctx, id, domain.KeepWorkAtRisk); err != nil {
		return &problem.Error{Code: problem.Internal, Message: "merge succeeded but removing the workspace failed", Err: err}
	}
	return nil
}

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
