// Package handlers holds the gin handlers backing the review endpoint: the
// branch-review read model, the windowed diff reads and the merge-strategy mutation.
package handlers

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ReviewUsecase is the branch-review surface the review handlers consume,
// satisfied by branchreview.Usecase.
type ReviewUsecase interface {
	// Get assembles the composite branch-review read model for a workspace.
	Get(
		ctx context.Context,
		wsID string,
	) (domain.BranchReview, error)
	// GetFiles returns the files-only branch-review summary for a workspace. commit scopes this
	// and every windowed read below to one commit against its parent; empty means the branch diff.
	GetFiles(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.ReviewFileSummary, error)
	// GetOutline returns the hunk geometry of the workspace's branch diff.
	GetOutline(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.FileOutline, error)
	// GetPatch streams one file's unified patch into w, returning the number of
	// patch lines written and whether it was cut short at maxLines.
	GetPatch(
		ctx context.Context,
		wsID string,
		commit string,
		path string,
		maxLines int,
		w io.Writer,
	) (int, bool, error)
	// GetFileSides returns one file's full old and new text for the diff.
	GetFileSides(
		ctx context.Context,
		wsID string,
		commit string,
		path string,
		oldPath string,
	) (gitdomain.ReviewFileSides, error)
	// SearchDiff finds query in the content of the workspace's branch diff.
	SearchDiff(
		ctx context.Context,
		wsID string,
		commit string,
		query string,
		opts gitdomain.SearchOpts,
	) ([]gitdomain.SearchHit, bool, error)
	// SetMergeStrategy updates the merge strategy for a workspace.
	SetMergeStrategy(
		ctx context.Context,
		wsID string,
		strategy gitdomain.MergeStrategy,
	) error
}

// Handlers serves the /v0 branch-review routes from the branch-review usecase.
type Handlers struct {
	reviewUsecase ReviewUsecase
}

// New builds the review Handlers from the branch-review usecase.
func New(
	reviewUsecase ReviewUsecase,
) *Handlers {
	return &Handlers{
		reviewUsecase: reviewUsecase,
	}
}

// workspaceID is the worktree this request acts on, read back from reqscope where the chat
// group's resolveChatWorktree middleware stashed it; no chat-scoped URL carries a workspace id.
func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	ws, _ := reqscope.Workspace(ctx)
	return ws.ID
}
