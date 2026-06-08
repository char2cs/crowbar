// Package handlers holds the gin handlers backing the review endpoint: the
// composite branch-review read model, merge-strategy mutation, and
// review-thread CRUD (open, reply, resolve/reopen) (02 §2.9, 09).
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ReviewUsecase is the branch-review surface the review handlers consume. It
// mirrors branchreview.Usecase so the concrete usecase satisfies it directly.
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
	// OpenThread opens a new review thread anchored to a file location.
	OpenThread(
		ctx context.Context,
		in branchreview.OpenThreadInput,
	) (domain.ReviewThread, error)
	// Reply appends a reply message to an existing review thread.
	Reply(
		ctx context.Context,
		threadID string,
		body string,
	) (domain.ReviewThread, error)
	// SetThreadResolved marks a review thread resolved or reopens it.
	SetThreadResolved(
		ctx context.Context,
		threadID string,
		resolved bool,
	) (domain.ReviewThread, error)
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
