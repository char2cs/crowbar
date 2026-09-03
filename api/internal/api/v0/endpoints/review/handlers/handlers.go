// Package handlers holds the gin handlers backing the review endpoint: the
// composite branch-review read model and merge-strategy mutation (02 §2.9, 09).
// Review-thread CRUD was promoted to the first-class /threads endpoint (W9).
package handlers

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ReviewUsecase is the branch-review surface the review handlers consume. It is
// satisfied by branchreview.Usecase, whose thread methods are now consumed by
// the dedicated threads endpoint rather than here.
type ReviewUsecase interface {
	// Get assembles the composite branch-review read model for a workspace.
	Get(
		ctx context.Context,
		wsID string,
	) (domain.BranchReview, error)
	// GetFiles returns the files-only branch-review summary for a workspace.
	//
	// commit scopes every windowed read below to one commit against its parent
	// instead of the branch; empty means the branch diff.
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

// workspaceID answers which worktree this request acts on, for either of the
// two groups review is currently mounted on (routes.go).
//
// On /v0/chats/:chatId/review... the chat group's resolveChatWorktree
// middleware has already resolved the chat's worktree and stashed the
// workspace on the context, so the answer is read back from reqscope — never
// resolved a second time per request, and never taken from a URL, because no
// chat-scoped URL carries a workspace id to take it from (spec law 1).
//
// The :wsId branch is the old workspace-scoped mount, unretired until spec §8
// step 6. When that mount goes, so does the branch, and this collapses to the
// reqscope read alone.
func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws.ID
	}
	return ctx.Param("wsId")
}
