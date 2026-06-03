package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReviewThreadUsecase is the public contract for review thread operations.
type ReviewThreadUsecase interface {
	Get(
		ctx context.Context,
		threadID string,
	) (domain.ReviewThread, error)

	List(
		ctx context.Context,
		taskID string,
		status string,
		phase string,
	) ([]domain.ReviewThread, error)

	Open(
		ctx context.Context,
		taskID string,
		file string,
		line int,
		content string,
	) (domain.ReviewThread, error)

	PostMessage(
		ctx context.Context,
		threadID string,
		role string,
		content string,
	) error

	ForceApprove(
		ctx context.Context,
		threadID string,
	) error
}

type reviewThreadUsecase struct {
	repo repositories.ReviewThread
}

// NewReviewThread wires a ReviewThread repository into a ReviewThreadUsecase.
func NewReviewThread(
	repo repositories.ReviewThread,
) ReviewThreadUsecase {
	return &reviewThreadUsecase{repo: repo}
}

func (u *reviewThreadUsecase) Get(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	return u.repo.Get(ctx, threadID)
}

func (u *reviewThreadUsecase) List(
	ctx context.Context,
	taskID string,
	status string,
	phase string,
) ([]domain.ReviewThread, error) {
	return u.repo.ListByTask(ctx, taskID, status, phase)
}

func (u *reviewThreadUsecase) Open(
	ctx context.Context,
	taskID string,
	file string,
	line int,
	content string,
) (domain.ReviewThread, error) {
	return u.repo.Open(
		ctx,
		taskID,
		nil,
		file,
		line,
		domain.ReviewPhaseHumanReview,
		"human",
		content,
	)
}

func (u *reviewThreadUsecase) PostMessage(
	ctx context.Context,
	threadID string,
	role string,
	content string,
) error {
	return u.repo.PostMessage(ctx, threadID, role, content)
}

func (u *reviewThreadUsecase) ForceApprove(
	ctx context.Context,
	threadID string,
) error {
	return u.repo.ForceApprove(ctx, threadID)
}
