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
	// GetFiles returns the files-only branch-review summary for a workspace: the
	// full changed-files picture (committed + working-tree, +N/-N counts) with no
	// line-level content, so the sidebar never fetches the diff body (Task 27).
	//
	// commit scopes every windowed read below to ONE commit against its parent
	// instead of the branch; empty means the branch diff. It is a parameter
	// rather than a separate set of methods because the two answer the same
	// question about a different pair of trees — the resolution differs, the
	// payload, the caching and the client do not. See resolveScopeRef.
	GetFiles(
		ctx context.Context,
		wsID string,
		commit string,
	) ([]gitdomain.ReviewFileSummary, error)
	// GetScope returns the ref this workspace's review diffs against, the
	// changed-file summary of that same diff, and its hunk geometry, from ONE ref
	// resolution.
	//
	// It exists because GetBase followed by GetFiles resolves the ref twice to
	// answer one question — what does this review cover — and each resolution is
	// up to three git subprocesses. See gitdomain.ReviewScope for why the parts
	// belong together on correctness grounds as well as cost.
	//
	// The geometry is the same outline GetOutline serves, through the same cache
	// and the same key, so an anchor checked against one is checked against the
	// other: a scope that reported ranges its own validator would then refuse
	// would be worse than reporting none.
	//
	// It takes the ALREADY-RESOLVED workspace rather than its id, unlike every
	// other method here, because its only caller is the agent tool surface and
	// that caller has just folded the same aggregate to authenticate: workspace
	// Get replays the whole event log and fires a background reconcile, so a
	// second read of a workspace resolved microseconds earlier pays both again to
	// learn nothing. Callers holding only an id want GetFiles, which resolves for
	// itself and shares this one's flight.
	GetScope(
		ctx context.Context,
		ws domain.Workspace,
	) (gitdomain.ReviewScope, error)
	// GetOutline returns the hunk geometry of the workspace's branch diff: per
	// file the `@@` shapes of its diff and no content at all. O(hunks) where Get
	// is O(lines), so the client can lay out a million-line diff before fetching
	// a single patch.
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
	// SearchDiff finds query in the content of the workspace's branch diff,
	// returning one hit per matching line plus whether opts.Limit cut the results
	// short. It is the server-side find-in-diff the client can no longer do
	// locally once it holds only a window of the patch.
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
	// GetBase returns the ref this workspace's review diffs against — the merge
	// base of the parent (or default) branch and HEAD, which is what every
	// /review surface actually uses. It is exported because the agent tool
	// surface must be able to tell a model what range it is reviewing; a model
	// left to guess reviews HEAD~1 or main and anchors every finding wrongly.
	GetBase(
		ctx context.Context,
		wsID string,
	) (string, error)
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

// GetBase returns the ref this workspace's review diffs against, so a caller
// outside this package (the agent tool surface) can learn what range a review
// covers instead of guessing HEAD~1 or main and anchoring findings against the
// wrong diff.
func (u *branchReviewUsecase) GetBase(
	ctx context.Context,
	wsID string,
) (string, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("branchreview: get base: workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveDiffRef(ctx, ws)
	if err != nil {
		return "", fmt.Errorf("branchreview: get base: resolve ref: %w", err)
	}
	return ref, nil
}

// resolveDiffRef returns the ref the review diffs against: the merge-base of the
// parent/default branch's CURRENT tip and HEAD, so the diff shows exactly this
// workspace's changes (committed + uncommitted) since it diverged — and stays
// correct as the branch is rebased onto newer base commits. It falls back to the
// recorded fork point only when the base branch cannot be resolved (no parent
// row, no merge-base), because a frozen fork point counts every commit the base
// branch gained after the branch was created, inflating the diff once the base
// advances past it.
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

// mergeBaseWithBase resolves the merge-base of a base branch and HEAD, probing
// BOTH origin/<base> and the local <base> and returning whichever merge-base is
// CLOSEST to HEAD (the descendant one). That is correct whichever ref the branch
// was built on: origin's merge-base when the local base lags origin, the local
// one when the local base is ahead of origin. Blindly preferring origin would
// re-inflate the diff by the local base branch's un-pushed commits. It returns ""
// when neither ref yields a merge-base, so the caller falls back to the fork point.
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

// closestToHead returns the candidate merge-base nearest HEAD — the descendant
// among them — resolved with MergeBase alone: if merge-base(best, mb) == best then
// best is an ancestor of mb, so mb sits closer to HEAD and wins. Divergent
// candidates (neither an ancestor of the other, a broken local/origin base) keep
// the first, origin-derived candidate.
//
// Identical candidates are answered without asking git. merge-base(X, X) is X by
// definition, so the comparison can only re-elect the candidate already held —
// and origin/<base> and <base> point at the same commit whenever the base branch
// is in sync with its remote, which is the ordinary state of a branch nobody has
// pushed to since the last fetch. That common case was spending a whole
// subprocess to compare a sha with itself.
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
		return "", fmt.Errorf("branch review: repo %s not found: %w", ws.RepoID, apperr.ErrNotFound)
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
