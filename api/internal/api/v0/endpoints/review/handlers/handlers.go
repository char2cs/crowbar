package handlers

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type ReviewUsecase interface {
	Get(
		ctx context.Context,
		wsID string,
	) (domain.BranchReview, error)
	GetFiles(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.ReviewFileSummary, error)
	GetOutline(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.FileOutline, error)
	GetPatch(
		ctx context.Context,
		wsID string,
		commit string,
		path string,
		maxLines int,
		w io.Writer,
	) (int, bool, error)
	GetFileSides(
		ctx context.Context,
		wsID string,
		commit string,
		path string,
		oldPath string,
	) (gitdomain.ReviewFileSides, error)
	SearchDiff(
		ctx context.Context,
		wsID string,
		commit string,
		query string,
		opts gitdomain.SearchOpts,
	) ([]gitdomain.SearchHit, bool, error)
	SetMergeStrategy(
		ctx context.Context,
		wsID string,
		strategy gitdomain.MergeStrategy,
	) error
}

type Handlers struct {
	reviewUsecase ReviewUsecase
}

func New(
	reviewUsecase ReviewUsecase,
) *Handlers {
	return &Handlers{
		reviewUsecase: reviewUsecase,
	}
}

func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	ws, _ := reqscope.Workspace(ctx)
	return ws.ID
}
