package branchreview

import (
	"context"
	"errors"
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
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

// Usecase assembles and mutates the branch-review surface (09).
type Usecase interface {
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
	repos      store.Store[domain.Repository, string]
	git        enginegit.Engine
	now        func() time.Time
}

// New builds the branch-review usecase wiring all collaborators.
func New(
	workspaces workspace.Workspace,
	threads reviewthread.ReviewThread,
	repos store.Store[domain.Repository, string],
	git enginegit.Engine,
	now func() time.Time,
) Usecase {
	return &branchReviewUsecase{
		workspaces: workspaces,
		threads:    threads,
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
	ref, err := u.resolveDiffRef(ctx, ws)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	diff, err := u.git.DiffAgainstRef(ctx, ws.WorktreePath, ref)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: diff: %w", err)
	}
	u.annotateUncommitted(ctx, ws, &diff)
	return u.assemble(ctx, ws, diff)
}

// resolveDiffRef returns the ref the review diffs against: the workspace's
// recorded fork point when known, else the merge-base of the parent/default
// branch and HEAD. This makes the diff show exactly this workspace's changes
// (committed + uncommitted) since it diverged.
func (u *branchReviewUsecase) resolveDiffRef(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	if ws.ForkPointSha != "" {
		return ws.ForkPointSha, nil
	}
	base, err := u.resolveBase(ctx, ws)
	if err != nil {
		return "", err
	}
	return u.git.MergeBase(ctx, ws.WorktreePath, base, "HEAD")
}

// annotateUncommitted marks each diff file as uncommitted when it has a matching
// entry in git status (staged or unstaged working-tree change). Status failures
// are non-fatal: the diff is still returned, just without the flags.
func (u *branchReviewUsecase) annotateUncommitted(
	ctx context.Context,
	ws domain.Workspace,
	diff *gitdomain.MultiFileDiff,
) {
	status, err := u.git.Status(ctx, ws.WorktreePath)
	if err != nil {
		return
	}
	dirty := make(map[string]bool, len(status.Files))
	for _, f := range status.Files {
		dirty[f.Path] = true
	}
	for i := range diff.Files {
		diff.Files[i].Uncommitted = dirty[diff.Files[i].FilePath]
	}
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
	for i := range threads {
		threads[i] = threads[i].NormalizedMessages()
	}
	return domain.BranchReview{
		Description:   "",
		MergeStrategy: ws.MergeStrategy,
		Diff:          diff,
		Threads:       threads,
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
	return thread.NormalizedMessages(), nil
}

func (u *branchReviewUsecase) Reply(
	ctx context.Context,
	threadID string,
	body string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Reply(ctx, threadID, uuid.NewString(), "", false, body, u.now())
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: reply: %w", asNotFound(err))
	}
	return thread.NormalizedMessages(), nil
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
	return thread.NormalizedMessages(), nil
}

func (u *branchReviewUsecase) reopenThread(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Reopen(ctx, threadID)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: reopen: %w", asNotFound(err))
	}
	return thread.NormalizedMessages(), nil
}
