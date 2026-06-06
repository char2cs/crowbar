package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchchat"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// asNotFound maps an engine-level not-found signal (asynx Get on a missing
// aggregate, or a command rejected because the target aggregate does not exist)
// to the shared apperr.ErrNotFound sentinel, leaving genuine internal failures
// untouched so handlers map them to 500 rather than 404.
func asNotFound(
	err error,
) error {
	if errors.Is(err, asynxModels.ErrNotFound) || errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("%w: %w", apperr.ErrNotFound, err)
	}
	return err
}

// BranchReviewUsecase assembles and mutates the branch-review surface (09).
type BranchReviewUsecase interface {
	// Get assembles the composite branch-review read model for a workspace (09 §2).
	Get(
		ctx context.Context,
		wsID string,
	) (domain.BranchReview, error)
	// SetMergeStrategy updates the merge strategy for a workspace (09 §4).
	SetMergeStrategy(
		ctx context.Context,
		wsID string,
		strategy gitdomain.MergeStrategy,
	) error
	// OpenThread opens a new review thread anchored to a file location (09 §3).
	OpenThread(
		ctx context.Context,
		in OpenThreadInput,
	) (domain.ReviewThread, error)
	// Reply appends a reply message to an existing review thread (09 §3).
	Reply(
		ctx context.Context,
		threadID string,
		body string,
	) (domain.ReviewThread, error)
	// SetThreadResolved marks a review thread resolved or reopens it (09 §3).
	SetThreadResolved(
		ctx context.Context,
		threadID string,
		resolved bool,
	) (domain.ReviewThread, error)
}

type branchReviewUsecase struct {
	workspaces workspace.Workspace
	threads    reviewthread.ReviewThread
	chats      chat.Chat
	repos      store.Store[domain.Repository, string]
	git        enginegit.Engine
	now        func() time.Time
}

// NewBranchReviewUsecase builds the branch-review usecase wiring all collaborators.
func NewBranchReviewUsecase(
	workspaces workspace.Workspace,
	threads reviewthread.ReviewThread,
	chats chat.Chat,
	repos store.Store[domain.Repository, string],
	git enginegit.Engine,
	now func() time.Time,
) BranchReviewUsecase {
	return &branchReviewUsecase{
		workspaces: workspaces,
		threads:    threads,
		chats:      chats,
		repos:      repos,
		git:        git,
		now:        now,
	}
}

func (u *branchReviewUsecase) Get(
	ctx context.Context,
	wsID string,
) (domain.BranchReview, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	base, err := u.resolveBase(ctx, ws)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: resolve base: %w", err)
	}
	diff, err := u.git.RangeDiff(ctx, ws.WorktreePath, base, ws.Branch)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: diff: %w", err)
	}
	return u.assemble(ctx, ws, diff)
}

func (u *branchReviewUsecase) resolveBase(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	if ws.ParentID != "" {
		parent, err := u.workspaces.Get(ctx, ws.ParentID)
		if err != nil {
			return "", asNotFound(err)
		}
		return parent.Branch, nil
	}
	repo, err := u.repos.FindByKey(ctx, ws.RepoID)
	if err != nil {
		return "", err
	}
	if repo == nil {
		return "", fmt.Errorf("branch review: repo %s not found: %w", ws.RepoID, apperr.ErrNotFound)
	}
	return repo.DefaultBranch, nil
}

func (u *branchReviewUsecase) assemble(
	ctx context.Context,
	ws domain.Workspace,
	diff gitdomain.MultiFileDiff,
) (domain.BranchReview, error) {
	threads, err := u.threads.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: threads: %w", err)
	}
	chats, err := u.chats.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: chats: %w", err)
	}
	return domain.BranchReview{
		Description:   "",
		MergeStrategy: ws.MergeStrategy,
		Diff:          diff,
		Threads:       threads,
		Conversations: branchchat.From(chats, u.now()),
	}, nil
}

func (u *branchReviewUsecase) SetMergeStrategy(
	ctx context.Context,
	wsID string,
	strategy gitdomain.MergeStrategy,
) error {
	if _, err := u.workspaces.SetMergeStrategy(ctx, wsID, strategy); err != nil {
		return fmt.Errorf("branch review: set merge strategy: %w", asNotFound(err))
	}
	return nil
}

func (u *branchReviewUsecase) OpenThread(
	ctx context.Context,
	in OpenThreadInput,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Open(ctx, reviewthread.OpenInput{
		ID:         uuid.NewString(),
		WsID:       in.WsID,
		FilePath:   in.FilePath,
		LineNumber: in.LineNumber,
		Side:       in.Side,
		MessageID:  uuid.NewString(),
		Body:       in.Body,
	}, u.now())
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: open thread: %w", err)
	}
	return thread, nil
}

func (u *branchReviewUsecase) Reply(
	ctx context.Context,
	threadID string,
	body string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Reply(ctx, threadID, uuid.NewString(), body, u.now())
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: reply: %w", asNotFound(err))
	}
	return thread, nil
}

func (u *branchReviewUsecase) SetThreadResolved(
	ctx context.Context,
	threadID string,
	resolved bool,
) (domain.ReviewThread, error) {
	if resolved {
		return u.resolveThread(ctx, threadID)
	}
	return u.reopenThread(ctx, threadID)
}

func (u *branchReviewUsecase) resolveThread(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Resolve(ctx, threadID)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: resolve: %w", asNotFound(err))
	}
	return thread, nil
}

func (u *branchReviewUsecase) reopenThread(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Reopen(ctx, threadID)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: reopen: %w", asNotFound(err))
	}
	return thread, nil
}
