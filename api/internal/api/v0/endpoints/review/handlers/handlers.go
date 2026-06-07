// Package handlers holds the gin handlers backing the branch-review endpoint:
// the composite review read model, the merge-strategy update, and the comment
// thread mutations (open, reply, resolve/reopen) (02 §2.9, 09).
//
// Every response is wrapped in the shared {success,error,data} envelope. Query
// responses (the composite read model) carry their payload in data; mutation
// responses carry the affected entity (the thread, or the updated strategy) in
// data. Usecase errors are mapped to HTTP status via libs.StatusAndMessage,
// which yields 404 for a genuine not-found aggregate and 500 for any other
// internal failure, so a real git/subprocess error is never masked as a 404.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ReviewService is the branch-review usecase surface the handlers consume. It
// mirrors branchreview.Usecase so the live usecase satisfies it directly.
type ReviewService interface {
	Get(
		ctx context.Context,
		wsID string,
	) (domain.BranchReview, error)
	SetMergeStrategy(
		ctx context.Context,
		wsID string,
		strategy gitdomain.MergeStrategy,
	) error
	OpenThread(
		ctx context.Context,
		in branchreview.OpenThreadInput,
	) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		threadID string,
		body string,
	) (domain.ReviewThread, error)
	SetThreadResolved(
		ctx context.Context,
		threadID string,
		resolved bool,
	) (domain.ReviewThread, error)
}

// Handlers serves the /v0 branch-review routes from the branch-review usecase.
type Handlers struct {
	review ReviewService
}

// New builds the branch-review Handlers from the branch-review usecase.
func New(
	review ReviewService,
) *Handlers {
	return &Handlers{
		review: review,
	}
}
