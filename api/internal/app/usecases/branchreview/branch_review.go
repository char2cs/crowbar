package branchreview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// ErrWorkspaceUnprovisioned is returned when a review targets a workspace with no worktree on disk, so
// there is nothing to diff. Handlers map it to 409: the row's state, not the request, is wrong.
var ErrWorkspaceUnprovisioned = problem.New(problem.Conflict, "usecases: this branch has no worktree yet")

// asNotFound maps an engine not-found signal to problem.ErrNotFound, leaving genuine internal failures
// untouched so they stay 500s.
func asNotFound(
	err error,
) error {
	if errors.Is(err, asynxModels.ErrNotFound) || errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("%w: %w", problem.ErrNotFound, err)
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
	// GetFiles returns the files-only review summary (no line content), so the sidebar never fetches the
	// diff body. A non-empty commit scopes the windowed reads to that commit against its parent: the same
	// question about a different pair of trees (see resolveScopeRef).
	GetFiles(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.ReviewFileSummary, error)
	// GetScope returns the review's ref, its changed-file summary and its hunk geometry from ONE resolution
	// (each costs up to three git subprocesses), through GetOutline's cache key so anchors agree. It takes
	// the already-resolved workspace because its agent-tool caller has just folded it; id callers use GetFiles.
	GetScope(
		ctx context.Context,
		ws domain.Workspace,
	) (gitdomain.ReviewScope, error)
	// GetOutline returns the branch diff's hunk geometry with no content: O(hunks), so the client can lay
	// out a huge diff before fetching any patch.
	GetOutline(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.FileOutline, error)
	// GetPatch streams one file's unified patch into w, returning the number of
	// patch lines written and whether it was cut short at maxLines. maxLines <= 0
	// means unlimited; an empty path is an invalid argument.
	GetPatch(
		ctx context.Context,
		wsID string,
		commit string,
		path string,
		maxLines int,
		w io.Writer,
	) (int, bool, error)
	// SearchDiff finds query in the branch diff, one hit per matching line plus whether opts.Limit cut the
	// results; the client holds only a window of the patch, so find runs here.
	SearchDiff(
		ctx context.Context,
		wsID string,
		commit string,
		query string,
		opts gitdomain.SearchOpts,
	) ([]gitdomain.SearchHit, bool, error)
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
	fileReads  singleflight.Group
	outlines   outlineCache
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
	return u.assemble(ctx, ws)
}

// resolveDiffRef returns the merge-base of the base branch's CURRENT tip and HEAD, so the diff stays
// correct across rebases. The recorded fork point is only a fallback: it inflates once the base advances.
func (u *branchReviewUsecase) resolveDiffRef(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	base, err := u.resolveBase(ctx, ws)
	if err == nil {
		if ref := u.mergeBaseWithBase(ctx, ws.WorktreePath, base); ref != "" {
			return ref, nil
		}
	}
	if ws.ForkPointSha != "" {
		return ws.ForkPointSha, nil
	}
	if err != nil {
		return "", err
	}
	return u.git.MergeBase(ctx, ws.WorktreePath, base, "HEAD")
}

// mergeBaseWithBase probes both origin/<base> and <base> and returns the merge-base closest to HEAD,
// right whichever ref the branch was built on; "" when neither yields one.
func (u *branchReviewUsecase) mergeBaseWithBase(
	ctx context.Context,
	worktreePath string,
	base string,
) string {
	bases := make([]string, 0, 2)
	for _, ref := range []string{"origin/" + base, base} {
		if mergeBase, err := u.git.MergeBase(ctx, worktreePath, ref, "HEAD"); err == nil && mergeBase != "" {
			bases = append(bases, mergeBase)
		}
	}
	return u.closestToHead(ctx, worktreePath, bases)
}

// closestToHead returns the candidate merge-base nearest HEAD (if merge-base(best, mb) == best, mb wins);
// divergent candidates keep the first. Identical candidates skip git, the common in-sync case.
func (u *branchReviewUsecase) closestToHead(
	ctx context.Context,
	worktreePath string,
	bases []string,
) string {
	if len(bases) == 0 {
		return ""
	}
	best := bases[0]
	for _, mb := range bases[1:] {
		if mb == best {
			continue
		}
		common, err := u.git.MergeBase(ctx, worktreePath, best, mb)
		if err == nil && common == best {
			best = mb
		}
	}
	return best
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
		return "", fmt.Errorf("branch review: repo %s not found: %w", ws.RepoID, problem.ErrNotFound)
	}
	return repo.DefaultBranch, nil
}

func (u *branchReviewUsecase) assemble(
	ctx context.Context,
	ws domain.Workspace,
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
	// No ProviderID or ChatID: this is the human/UI path, and a person has neither
	// a vendor CLI behind them nor an originating agent conversation.
	thread, err := u.threads.Reply(ctx, reviewthread.ReplyInput{
		ID:        threadID,
		MessageID: uuid.NewString(),
		Body:      body,
	}, u.now())
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
