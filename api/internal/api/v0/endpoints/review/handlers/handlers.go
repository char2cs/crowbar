// Package handlers holds the gin handlers backing the review endpoint: the
// composite branch-review read model and merge-strategy mutation (02 §2.9, 09).
// Review-thread CRUD was promoted to the first-class /threads endpoint (W9).
package handlers

import (
	"context"

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
