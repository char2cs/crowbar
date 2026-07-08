package worktree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// CreateChildInput carries the fields needed to create a worktree-backed child.
type CreateChildInput struct {
	RepoID       string
	ProjectID    string
	RepoPath     string
	RemoteURL    string // git remote origin URL; required for worktree path derivation
	Branch       string
	ParentID     string
	ParentBranch string
	// ForceLocked overrides the provider-driven locked check and marks the
	// workspace locked regardless of whether the branch is protected. Useful
	// when the caller knows the workspace should be immutable (e.g. the repo's
	// main branch workspace adopted without a git provider).
	ForceLocked bool
}

// Usecase orchestrates the worktree hierarchy (07): worktree-backed
// create, local child→parent merge (all three strategies), re-parenting, and
// cascade delete. It composes the git-engine primitives with 3A's Workspace
// Asynx commands; it forks neither. All hierarchy guards live here.
type Usecase interface {
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	// RetryProvision re-runs holder resolution + provisioning for an existing
	// placeholder (same id): on success it attaches the worktree, records the
	// fork point, and clears HeldByPath (status stays locked). Returns
	// ErrBranchStillHeld when the branch is still live-held (spec §3.3).
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees a live holder off the placeholder's branch (with the
	// caller's consent), clears the home row's branch when the holder is the repo
	// home, then Retry-provisions in place (spec §3.5/§3.7).
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
}

// TerminalReaper is the narrow terminal-engine surface the cascade delete uses to
// terminate a workspace's live PTY sessions before its worktree is removed, so the
// shell processes, their fds, and the per-session ring buffers don't leak.
type TerminalReaper interface {
	ListSessionsForWorkspace(workspaceID string) []string
	Kill(ctx context.Context, sessionID string) error
}

type worktreeUsecase struct {
	workspaces  workspace.Workspace
	git         enginegit.Engine
	provider    engineprovider.Engine
	repos       store.Store[domain.Repository, string]
	now         func() time.Time
	crowbarHome func() (string, error)
	terminals   TerminalReaper
}

// Option configures optional worktreeUsecase dependencies without widening the
// New signature (it has many test call sites).
type Option func(*worktreeUsecase)

// WithTerminalReaper wires the terminal engine so cascade delete can kill a
// workspace's PTY sessions. Without it, terminal cleanup is skipped (tests).
func WithTerminalReaper(r TerminalReaper) Option {
	return func(u *worktreeUsecase) { u.terminals = r }
}

// New builds the hierarchy usecase. repos resolves a workspace's repository main
// path (via RepoID) so worktree removal and branch deletion run against the repo,
// never against a child's own worktree (07 §5).
func New(
	workspaces workspace.Workspace,
	git enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
	opts ...Option,
) Usecase {
	u := &worktreeUsecase{
		workspaces:  workspaces,
		git:         git,
		provider:    provider,
		repos:       repos,
		now:         now,
		crowbarHome: crowbarHome,
	}
	for _, o := range opts {
		o(u)
	}
	return u
}

//nolint:gocyclo // orchestrates create with intricate worktree/branch rollback paths; splitting risks the rollback invariants
func (u *worktreeUsecase) CreateChild(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	// Repos with no on-disk path (e.g. virtual/test repos) skip all git
	// operations and create a workspace row directly.
	if in.RepoPath == "" {
		return u.workspaces.Create(ctx, workspace.CreateInput{
			ID:        uuid.NewString(),
			RepoID:    in.RepoID,
			ProjectID: in.ProjectID,
			Branch:    in.Branch,
			ParentID:  in.ParentID,
			Protected: in.ForceLocked,
		}, u.now())
	}
	// At most one MANAGED (non-default) workspace per (repo, branch). The default
	// workspace (the imported repo folder) never counts — see branchWorkspaceExists.
	exists, err := u.branchWorkspaceExists(ctx, in.RepoID, in.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	if exists {
		return domain.Workspace{}, fmt.Errorf("%w (repo %s, branch %q)", ErrBranchWorkspaceExists, in.RepoID, in.Branch)
	}
	// When ParentID is empty and the requested branch matches the repo's default
	// branch, the FIRST such create adopts the existing repo path as the default
	// workspace instead of creating a new git worktree. A repeat (e.g. importing
	// the default branch via the branch panel after the folder is already
	// adopted) must NOT create a second default — fall through to the managed
	// worktree path, where git rejects checking the branch out a second time.
	if in.ParentID == "" && in.Branch == in.ParentBranch { //nolint:nestif // adopt-vs-managed create branching; flattening risks the adoption invariant
		adopted, err := u.mainWorktreeAdopted(ctx, in.RepoID)
		if err != nil {
			return domain.Workspace{}, err
		}
		if !adopted {
			return u.adoptMainWorktree(ctx, in)
		}
	}
	wsID := uuid.NewString()
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: crowbar home: %w", err)
	}
	// Resolve the locked flag BEFORE creating the worktree so a provider hiccup
	// can't leave an orphaned worktree+branch on disk (it has no on-disk effect).
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: locked: %w", err)
	}
	path, err := u.deriveWorktreePath(ctx, home, in.ProjectID, in.RepoID, in.RemoteURL, in.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	detached := false
	startSha, err := u.addWorktree(ctx, in, path)
	if err != nil { //nolint:nestif // main-folder detach-and-retry rollback; flattening risks the detach/reattach invariant
		// git refuses a worktree on a branch that's already checked out. If the
		// holder is the repo's MAIN folder (the unmanaged default workspace),
		// detach it — its branch is incidental — to free the branch, and retry
		// once. A branch held by another managed worktree is left to fail (the
		// one-managed-per-branch guard already covers that case).
		if outcome, hErr := holder.Resolve(ctx, u.git, in.RepoPath, in.Branch, home); hErr == nil && outcome.Kind == holder.HeldByHome {
			if dErr := u.git.DetachWorktree(ctx, in.RepoPath); dErr == nil {
				detached = true
				startSha, err = u.addWorktree(ctx, in, path)
			}
		}
		if err != nil {
			u.reattachMain(ctx, detached, in.RepoPath, in.Branch)
			return domain.Workspace{}, err
		}
	}
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           wsID,
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Protected:    locked || in.ForceLocked,
	}, u.now())
	if err != nil { //nolint:nestif // orphan worktree+branch cleanup after a failed row create; flattening risks the rollback ordering
		// The worktree + branch are on disk but the workspace row never landed.
		// Clean them up best-effort so a fresh-wsID retry isn't blocked by the
		// orphaned branch and the worktree dir doesn't dangle forever.
		if rmErr := u.git.WorktreeRemove(ctx, in.RepoPath, path); rmErr != nil {
			slog.WarnContext(ctx, "create child: cleanup worktree after failed create",
				"worktree", path, "err", rmErr)
		}
		// Re-attach the main folder FIRST (if we detached it): this restores the
		// folder AND re-checks-out the branch, so the force-delete below cannot
		// destroy an adopted (pre-existing) branch like the default branch.
		u.reattachMain(ctx, detached, in.RepoPath, in.Branch)
		if delErr := u.git.ForceDeleteBranch(ctx, in.RepoPath, in.Branch); delErr != nil {
			slog.WarnContext(ctx, "create child: cleanup branch after failed create",
				"branch", in.Branch, "err", delErr)
		}
		return domain.Workspace{}, err
	}
	return ws, nil
}

// deriveWorktreePath returns the human-readable git worktree directory for a
// branch — <home>/projects/<project>/<slug>/<branch> (spec §3.9) — with the repo
// slug resolved from its remote identity. It rejects a candidate that collides
// case-insensitively with an existing sibling worktree (spec §3.9, decision 13),
// surfacing the clash as apperr.ErrInvalidArgument.
func (u *worktreeUsecase) deriveWorktreePath(
	ctx context.Context,
	home string,
	projectID string,
	repoID string,
	remoteURL string,
	branch string,
) (string, error) {
	slug, err := u.resolveSlug(ctx, repoID, remoteURL)
	if err != nil {
		return "", fmt.Errorf("resolve worktree slug: %w", err)
	}
	path, err := worktreepath.Derive(home, projectID, slug, branch)
	if err != nil {
		return "", err
	}
	siblings, err := siblingWorktreePaths(home, projectID, slug)
	if err != nil {
		return "", fmt.Errorf("scan sibling worktrees: %w", err)
	}
	if clashErr := worktreepath.DetectClash(siblings, path); clashErr != nil {
		return "", fmt.Errorf("%w: %v", apperr.ErrInvalidArgument, clashErr)
	}
	return path, nil
}

// resolveSlug resolves the repo's on-disk identity slug (spec §3.9). It always
// loads the repo row so the no-remote / unparseable-URL fallback can reach the
// repo NAME: RemoteSlug degrades a remote that does not encode a host/owner/repo
// identity (a local bare path, a nameless remote) to Repository.Name, and a
// caller-supplied remoteURL carries no name — so resolving from the URL alone
// would fold such a remote to an EMPTY slug and fail Derive. A caller that
// carries the remote URL (Create) has it applied over the loaded row so the
// parse still prefers the caller's value while the name stays available as the
// fallback.
func (u *worktreeUsecase) resolveSlug(
	ctx context.Context,
	repoID string,
	remoteURL string,
) (string, error) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return "", err
	}
	if repo == nil {
		return "", apperr.ErrNotFound
	}
	if remoteURL != "" {
		repo.RemoteURL = remoteURL
	}
	return worktreepath.RemoteSlug(*repo), nil
}

// siblingWorktreePaths lists the existing branch-leaf worktrees under a repo's
// derived slug directory, so a create can reject a case-insensitive path clash.
// A not-yet-created slug directory yields no siblings.
func siblingWorktreePaths(
	home string,
	projectID string,
	slug string,
) ([]string, error) {
	parent := filepath.Join(home, "projects", projectID, slug)
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(parent, entry.Name()))
	}
	return paths, nil
}

// addWorktree applies the spec-§3 checkout-vs-create decision and returns the
// fork-point SHA the workspace aggregate should record.
//
// If the requested branch already exists on the `origin` remote, it is fetched
// and checked out into a fresh worktree (tracking origin/<branch>); the fork
// point is the resolved tip of origin/<branch>. Otherwise the branch is created
// locally from ParentBranch, and the fork point is the start SHA reported by
// WorktreeAddBranch. The two paths have DIFFERENT fork-point semantics: the
// checkout path's fork point is the remote tip (so later merge/reparent math
// diffs against what the remote already contains), while the create path's fork
// point is exactly where the new branch diverged from its parent.
func (u *worktreeUsecase) addWorktree(
	ctx context.Context,
	in CreateChildInput,
	path string,
) (string, error) {
	// Fast-forward the parent branch to match origin before the child branches
	// off it. This avoids the common scenario where the parent is stale locally
	// and the new branch immediately diverges from what the remote already has.
	// Best-effort: a network outage or diverged parent must not block branch
	// creation — the user can pull the parent manually afterward.
	if in.ParentBranch != "" { //nolint:nestif // best-effort parent fast-forward before branching; guards are load-bearing
		if parentOnRemote, err := u.git.RemoteBranchExists(ctx, in.RepoPath, in.ParentBranch); err == nil && parentOnRemote {
			if err := u.git.FastForwardBranch(ctx, in.RepoPath, in.ParentBranch); err != nil {
				slog.WarnContext(ctx, "create child: could not fast-forward parent; branching from local tip",
					"parent", in.ParentBranch, "err", err)
			}
		}
	}
	exists, err := u.git.RemoteBranchExists(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return "", fmt.Errorf("create child: remote branch exists: %w", err)
	}
	if exists {
		return u.checkoutRemoteBranch(ctx, in, path)
	}
	startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch)
	if err != nil {
		return "", fmt.Errorf("create child: worktree add: %w", err)
	}
	return startSha, nil
}

// checkoutRemoteBranch fast-forwards the local copy of an existing remote
// branch and adds a worktree checking it out. The fork point is the resolved
// origin/<branch> tip. Using FastForwardBranch (rather than FetchRef) ensures
// the worktree starts at the same commit as origin, not a stale local ref.
func (u *worktreeUsecase) checkoutRemoteBranch(
	ctx context.Context,
	in CreateChildInput,
	path string,
) (string, error) {
	if err := u.git.FastForwardBranch(ctx, in.RepoPath, in.Branch); err != nil {
		return "", fmt.Errorf("create child: fast-forward branch: %w", err)
	}
	forkPoint, err := u.git.RevParse(ctx, in.RepoPath, "origin/"+in.Branch)
	if err != nil {
		return "", fmt.Errorf("create child: resolve remote ref: %w", err)
	}
	if err := u.git.WorktreeAdd(ctx, in.RepoPath, path, in.Branch); err != nil {
		return "", fmt.Errorf("create child: worktree checkout: %w", err)
	}
	return forkPoint, nil
}

// adoptMainWorktree registers the repository's main worktree as a workspace
// without creating a new git worktree. It is used when the requested branch is
// the repository's default branch and there is no parent workspace.
func (u *worktreeUsecase) adoptMainWorktree(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	startSha, err := u.git.RevParse(ctx, in.RepoPath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: adopt main worktree: rev-parse HEAD: %w", err)
	}
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: adopt main worktree: locked: %w", err)
	}
	return u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: in.RepoPath,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Protected:    locked || in.ForceLocked,
		// The adopted main worktree IS the repo's default workspace. Marking it
		// keeps IsDefault reliable for the one-managed-workspace-per-branch guard,
		// which must never count the default.
		IsDefault: true,
	}, u.now())
}

// branchWorkspaceExists reports whether a non-deleted workspace already holds
// this branch in the repo. A branch can be checked out in at most one worktree,
// so the repo keeps at most one workspace per branch.
func (u *worktreeUsecase) branchWorkspaceExists(
	ctx context.Context,
	repoID string,
	branch string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, fmt.Errorf("create child: list workspaces: %w", err)
	}
	for _, w := range all {
		// Only Crowbar-MANAGED workspaces count. The default workspace is the
		// imported repo folder itself — an unmanaged checkout that merely happens
		// to sit on some branch; Crowbar does not own that branch, so it must not
		// block importing the same branch as a real managed workspace.
		if w.RepoID == repoID &&
			w.Branch == branch &&
			!w.IsDefault &&
			w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}

// mainWorktreeAdopted reports whether the repo's main worktree (the default
// workspace) is already adopted, so adoptMainWorktree runs at most once and a
// repeat attempt cannot persist a duplicate default row.
func (u *worktreeUsecase) mainWorktreeAdopted(
	ctx context.Context,
	repoID string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, fmt.Errorf("create child: list workspaces: %w", err)
	}
	for _, w := range all {
		if w.RepoID == repoID && w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}

// reattachMain re-checks-out `branch` in the main folder after a detach, to roll
// back a failed managed-worktree create (or to restore the folder when the
// managed worktree holding its branch is removed). Best-effort: a failure is
// logged, never fatal.
func (u *worktreeUsecase) reattachMain(
	ctx context.Context,
	detached bool,
	repoPath string,
	branch string,
) {
	if !detached {
		return
	}
	if err := u.git.CheckoutBranch(ctx, repoPath, branch); err != nil {
		slog.WarnContext(ctx, "create child: re-attach main worktree after rollback",
			"repo", repoPath, "branch", branch, "err", err)
	}
}

func (u *worktreeUsecase) resolveLocked(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	protected, err := u.provider.ProtectedBranches(ctx, repoPath)
	if err != nil {
		return false, err
	}
	for _, b := range protected {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}

func (u *worktreeUsecase) MergeIntoParent(
	ctx context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (MergeResult, error) {
	child, parent, err := u.loadMergePair(ctx, childID)
	if err != nil {
		return MergeResult{}, err
	}
	if guardErr := u.guardMerge(ctx, child, parent, strategy); guardErr != nil {
		return MergeResult{}, guardErr
	}
	if mergeErr := u.runMerge(ctx, child, parent, strategy); mergeErr != nil {
		return u.handleMergeError(ctx, child, parent, strategy, mergeErr)
	}
	return u.finalizeMerge(ctx, child, parent)
}

func (u *worktreeUsecase) loadMergePair(
	ctx context.Context,
	childID string,
) (domain.Workspace, domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, domain.Workspace{}, fmt.Errorf("merge: get child: %w", err)
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, domain.Workspace{}, fmt.Errorf("merge: get parent: %w", err)
	}
	return child, parent, nil
}

func (u *worktreeUsecase) guardMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
) error {
	if parent.WorktreePath == "" {
		return ErrParentUnprovisioned
	}
	if parent.Status == domain.WorkspaceStatusLocked {
		return ErrParentLocked
	}
	if strategy != gitdomain.MergeStrategyRebase {
		return nil
	}
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("merge: leaf check: %w", err)
	}
	if hasKids {
		return ErrRebaseNonLeaf
	}
	return nil
}

func (u *worktreeUsecase) runMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
) error {
	if strategy == gitdomain.MergeStrategySquash {
		subject := fmt.Sprintf("Squash merge %s", child.Branch)
		return u.git.MergeSquash(ctx, parent.WorktreePath, child.Branch, subject)
	}
	if strategy == gitdomain.MergeStrategyRebase {
		return u.runRebaseMerge(ctx, child, parent)
	}
	return u.git.Merge(ctx, parent.WorktreePath, child.Branch)
}

func (u *worktreeUsecase) runRebaseMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
) error {
	return u.git.RebaseThenFFMerge(
		ctx,
		child.WorktreePath,
		parent.Branch,
		parent.WorktreePath,
		child.Branch,
	)
}

func (u *worktreeUsecase) handleMergeError(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
	mergeErr error,
) (MergeResult, error) {
	if !errors.Is(mergeErr, enginegit.ErrConflict) {
		return MergeResult{}, fmt.Errorf("merge: run: %w", mergeErr)
	}
	// Try-then-warn (consistent with replayAndReparent): a conflicting
	// merge-into-parent must NEVER leave a worktree stuck. The merge runs in the
	// PARENT for the squash/plain-merge strategies and in the CHILD for the
	// rebase strategy, so abort the in-progress op in whichever worktree holds
	// it. The conflict is then surfaced only as the child's pr-conflicts state;
	// the user resolves it via "Rebase onto parent" (which keeps a resolvable
	// rebase in the child's OWN worktree) and re-runs the merge once clean.
	abortPath := parent.WorktreePath
	abortWS := parent.ID
	if strategy == gitdomain.MergeStrategyRebase {
		abortPath = child.WorktreePath
		abortWS = child.ID
	}
	if abortErr := u.git.OperationAbort(ctx, abortPath); abortErr != nil { //nolint:nestif // stuck-worktree recovery after a failed conflict abort; flattening risks the recovery path
		slog.WarnContext(ctx, "merge: abort after conflict failed; worktree may be stuck",
			"workspace_id", child.ID, "abort_path", abortPath, "err", abortErr)
		// The abort failed, so the worktree that holds the op is NOT clean — its
		// index is still mid-merge. When that worktree is the PARENT (the child is
		// flagged below regardless), flag the parent pr-conflicts too so the stuck
		// state is VISIBLE and recoverable (the user can abort it from its panel)
		// rather than a silent brick reported as merge-pending success (R7).
		if abortWS != child.ID {
			if _, perr := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
				ID:           abortWS,
				HasConflicts: true,
			}, u.now()); perr != nil {
				slog.WarnContext(ctx, "merge: flag stuck parent after failed abort",
					"workspace_id", abortWS, "err", perr)
			}
		}
	}
	// A local merge/rebase conflict transitions the child to Status=pr-conflicts
	// (07 §3.1, 00 §6.1): the HasConflicts sync input drives the status enum.
	_, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           child.ID,
		HasConflicts: true,
	}, u.now())
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge: set conflicts: %w", err)
	}
	return MergeResult{ConflictsPending: true}, nil
}

func (u *worktreeUsecase) finalizeMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
) (MergeResult, error) {
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge: parent tip: %w", err)
	}
	if _, err := u.workspaces.UpdateForkPoint(ctx, child.ID, tip); err != nil {
		return MergeResult{}, fmt.Errorf("merge: update fork point: %w", err)
	}
	if _, err := u.resyncSummary(ctx, parent.ID, parent.WorktreePath, parent.ForkPointSha); err != nil {
		slog.WarnContext(ctx, "merge: parent summary resync failed; read-model will self-correct", "workspace_id", parent.ID, "err", err)
	}
	if _, err := u.resyncSummary(ctx, child.ID, child.WorktreePath, tip); err != nil {
		slog.WarnContext(ctx, "merge: child summary resync failed; read-model will self-correct", "workspace_id", child.ID, "err", err)
	}
	return MergeResult{ParentTipSha: tip}, nil
}

// resyncSummary recomputes a workspace's working-tree summary from git and
// pushes it into the read model so Added/Deleted/HasCommits/HasConflicts reflect
// the post-merge state of both the parent and the kept child. It returns the
// hasConflicts it computed so callers can act on it without a second
// WorkingTreeSummary call.
func (u *worktreeUsecase) resyncSummary(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (bool, error) {
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(ctx, worktreePath, forkPointSha)
	if err != nil {
		return false, fmt.Errorf("merge: resync summary: %w", err)
	}
	_, err = u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           id,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	}, u.now())
	if err != nil {
		return false, fmt.Errorf("merge: resync summary: sync: %w", err)
	}
	return hasConflicts, nil
}

func (u *worktreeUsecase) Reparent(
	ctx context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get child: %w", err)
	}
	newParent, err := u.workspaces.Get(ctx, newParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get new parent: %w", err)
	}
	if guardErr := u.guardReparent(ctx, child, newParent); guardErr != nil {
		return domain.Workspace{}, guardErr
	}
	tip, err := u.git.RevParse(ctx, newParent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: new parent tip: %w", err)
	}
	return u.replayAndReparent(ctx, child, newParentID, tip)
}

// replayAndReparent tries to rebase the child onto the new parent's tip, then
// settles the move. Try-then-warn: a CLEAN rebase moves the child onto the new
// parent and integrates it; a CONFLICT never leaves the rebase stuck — it is
// ABORTED back to a clean worktree, yet the child is STILL moved under the new
// parent ("moved on paper, not yet integrated"). The predicted-conflict overlay
// (mergeConflicts) marks it; the user finishes the rebase on their own terms.
func (u *worktreeUsecase) replayAndReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	tip string,
) (domain.Workspace, error) {
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, child.ForkPointSha, child.Branch)
	if rebaseErr == nil {
		// Clean: the branch is genuinely based on the new parent's tip now.
		return u.settleReparent(ctx, child, newParentID, tip)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("reparent: rebase onto: %w", rebaseErr)
	}
	// Conflict: abort the rebase so the worktree returns to clean (never the stuck
	// mid-rebase mess), but keep the move.
	if abortErr := u.git.OperationAbort(ctx, child.WorktreePath); abortErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: abort after conflict: %w", abortErr)
	}
	// Fork point = merge-base of the branch and the new parent, so diffs/summaries
	// read against the new parent's lineage until a clean rebase finalizes it.
	base, baseErr := u.git.MergeBase(ctx, child.WorktreePath, tip, child.Branch)
	if baseErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: merge-base: %w", baseErr)
	}
	return u.settleReparent(ctx, child, newParentID, base)
}

// settleReparent persists the child's new parent + fork point and resyncs its
// working-tree summary (best-effort, like finalizeMerge) so a prior conflict
// status clears and the diff stats reflect the new base.
func (u *worktreeUsecase) settleReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	forkPoint string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Reparent(ctx, child.ID, newParentID, forkPoint, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: persist: %w", err)
	}
	if _, syncErr := u.resyncSummary(ctx, child.ID, child.WorktreePath, forkPoint); syncErr != nil {
		slog.WarnContext(ctx, "reparent: child summary resync failed; read-model will self-correct",
			"workspace_id", child.ID, "err", syncErr)
	}
	return ws, nil
}

// RebaseOntoParent is the user-initiated "finish the move" action for a
// moved-but-conflicting child: it rebases the child onto its CURRENT parent's
// tip. Unlike the automatic reparent (which aborts on conflict), this KEEPS a
// conflicting rebase in progress so the user can resolve it with the standard
// conflict tooling; a clean rebase integrates the child. The intended fork point
// (the parent tip) is persisted up front so the branch reads correctly once
// resolved.
func (u *worktreeUsecase) RebaseOntoParent(
	ctx context.Context,
	childID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get child: %w", err)
	}
	if child.ParentID == "" {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: workspace has no parent")
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get parent: %w", err)
	}
	if parent.WorktreePath == "" {
		return domain.Workspace{}, ErrParentUnprovisioned
	}
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: parent tip: %w", err)
	}
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, child.ForkPointSha, child.Branch)
	if rebaseErr == nil {
		// Clean: the child is genuinely integrated onto the parent.
		return u.settleReparent(ctx, child, child.ParentID, tip)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: %w", rebaseErr)
	}
	// Conflict: KEEP the rebase in progress (the user resolves it with the
	// standard conflict tooling). Persist the intended fork point now so the
	// branch reads correctly once resolved, and surface the conflict via status.
	if _, err := u.workspaces.Reparent(ctx, child.ID, child.ParentID, tip, u.now()); err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: persist: %w", err)
	}
	ws, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           child.ID,
		HasConflicts: true,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: set conflicts: %w", err)
	}
	return ws, nil
}

// RetryProvision re-provisions a placeholder workspace in place (spec §3.3).
func (u *worktreeUsecase) RetryProvision(
	ctx context.Context,
	wsID string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: get workspace: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: repo path: %w", err)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: crowbar home: %w", err)
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, ws.Branch, home)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: resolve holder: %w", err)
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal {
		return domain.Workspace{}, fmt.Errorf("%w (%s at %s)", ErrBranchStillHeld, ws.Branch, outcome.HeldByPath)
	}
	path, err := u.deriveWorktreePath(ctx, home, ws.ProjectID, ws.RepoID, "", ws.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	startSha, err := u.materializeProtectedWorktree(ctx, repoPath, ws.Branch, path)
	if err != nil {
		return domain.Workspace{}, err
	}
	provisioned, err := u.workspaces.ProvisionInPlace(ctx, ws.ID, path, startSha)
	if err != nil {
		// The worktree is on disk but the row never landed — clean it up so a
		// later retry isn't blocked by the orphaned worktree.
		if rmErr := u.git.WorktreeRemove(ctx, repoPath, path); rmErr != nil {
			slog.WarnContext(ctx, "retry provision: cleanup worktree after failed provision",
				"worktree", path, "err", rmErr)
		}
		return domain.Workspace{}, fmt.Errorf("retry provision: persist: %w", err)
	}
	return provisioned, nil
}

// materializeProtectedWorktree fast-forwards the protected branch best-effort
// then checks it out into a fresh worktree at path, returning the branch tip SHA.
// Mirrors the import path's addProtectedWorktree.
func (u *worktreeUsecase) materializeProtectedWorktree(
	ctx context.Context,
	repoPath string,
	branch string,
	path string,
) (string, error) {
	if exists, err := u.git.RemoteBranchExists(ctx, repoPath, branch); err == nil && exists {
		if ffErr := u.git.FastForwardBranch(ctx, repoPath, branch); ffErr != nil {
			slog.WarnContext(ctx, "retry provision: could not fast-forward protected branch; using local tip",
				"branch", branch, "err", ffErr)
		}
	}
	if err := u.git.WorktreeAdd(ctx, repoPath, path, branch); err != nil {
		return "", fmt.Errorf("retry provision: worktree add: %w", err)
	}
	sha, err := u.git.RevParse(ctx, repoPath, "refs/heads/"+branch)
	if err != nil {
		return "", nil // fork point non-essential; the worktree is valid
	}
	return sha, nil
}

// DetachHolder frees a live holder off a placeholder's branch with consent, then
// re-provisions in place. When the holder is the repo home it also clears the
// home row's branch (spec §3.5/§3.7). A detach failure returns cleanly — no
// ClearBranch, no Retry — so there is never partial state.
func (u *worktreeUsecase) DetachHolder(
	ctx context.Context,
	wsID string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: get workspace: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: repo path: %w", err)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: crowbar home: %w", err)
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, ws.Branch, home)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: resolve holder: %w", err)
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal { //nolint:nestif // consent-gated holder detach + home-branch clear; flattening risks the detach/clear ordering
		if dErr := u.git.DetachWorktree(ctx, outcome.HeldByPath); dErr != nil {
			return domain.Workspace{}, fmt.Errorf("detach holder: detach %s: %w", outcome.HeldByPath, dErr)
		}
		if outcome.Kind == holder.HeldByHome {
			homeID, ok, hErr := u.repoHomeWorkspaceID(ctx, ws.RepoID)
			if hErr != nil {
				return domain.Workspace{}, fmt.Errorf("detach holder: find home: %w", hErr)
			}
			if ok {
				if _, cErr := u.workspaces.ClearBranch(ctx, homeID); cErr != nil {
					return domain.Workspace{}, fmt.Errorf("detach holder: clear home branch: %w", cErr)
				}
			}
		}
	}
	return u.RetryProvision(ctx, wsID)
}

// repoHomeWorkspaceID returns the id of the repo's default (home) workspace.
func (u *worktreeUsecase) repoHomeWorkspaceID(
	ctx context.Context,
	repoID string,
) (string, bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return "", false, err
	}
	for _, w := range all {
		if w.RepoID == repoID && w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			return w.ID, true, nil
		}
	}
	return "", false, nil
}

func (u *worktreeUsecase) guardReparent(
	ctx context.Context,
	child domain.Workspace,
	newParent domain.Workspace,
) error {
	if child.ID == newParent.ID {
		return ErrSelfParent
	}
	if newParent.WorktreePath == "" {
		return ErrParentUnprovisioned
	}
	if newParent.Status == domain.WorkspaceStatusLocked {
		return ErrNewParentLocked
	}
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("reparent: leaf check: %w", err)
	}
	if hasKids {
		return ErrChildHasChildren
	}
	return nil
}

func (u *worktreeUsecase) DeleteCascade(
	ctx context.Context,
	rootID string,
) error {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("delete cascade: list: %w", err)
	}
	index := indexByID(all)
	root, ok := index[rootID]
	if !ok {
		return fmt.Errorf("delete cascade: workspace %s: %w", rootID, apperr.ErrNotFound)
	}
	if root.Status == domain.WorkspaceStatusLocked {
		return ErrWorkspaceLocked
	}
	order := cascade.Plan(rootID, nodesFrom(all))
	for _, id := range order {
		if removeErr := u.removeOne(ctx, index[id]); removeErr != nil {
			return fmt.Errorf("delete cascade: remove %s: %w", id, removeErr)
		}
	}
	return nil
}

func (u *worktreeUsecase) removeOne(
	ctx context.Context,
	ws domain.Workspace,
) error {
	// Kill the workspace's live PTY sessions FIRST, before the worktree is removed.
	// They are keyed by workspace id and otherwise survive the delete as orphaned
	// shell processes with a now-deleted CWD, leaking fds and ring-buffer memory on
	// every workspace/cascade delete. Best-effort: a kill failure must not abort the
	// cascade. Runs even when the repo path can't be resolved below.
	u.reapTerminals(ctx, ws.ID)

	repo, err := u.repos.FindByKey(ctx, ws.RepoID)
	if err != nil || repo == nil {
		// Can't resolve the repo — still drop the read-model row so the cascade
		// doesn't leave a ghost workspace pointing at an unreachable worktree.
		slog.WarnContext(ctx, "cascade: repo unresolved; dropping row best-effort",
			"ws", ws.ID, "err", err)
		return u.workspaces.Delete(ctx, ws.ID)
	}
	repoPath := repo.Path
	// A placeholder (empty WorktreePath) has no worktree, no managed branch
	// checkout, and its real branch must never be git-touched: drop the row only.
	// Defense-in-depth — the locked status already blocks DeleteCascade, but a
	// direct removeOne must not run git against "" or -D the protected branch.
	if ws.WorktreePath == "" {
		return u.workspaces.Delete(ctx, ws.ID)
	}
	// Best-effort git teardown: a failure here (branch checked out elsewhere, a
	// transient index lock, an already-removed worktree) must NOT abort the cascade
	// or leave a GHOST row pointing at a gone worktree — that breaks every future op
	// on it and makes a re-run cascade fail too. Log and continue; the row is always
	// dropped, and an orphaned worktree on disk is reaped by `git worktree prune`.
	if removeErr := u.git.WorktreeRemove(ctx, repoPath, ws.WorktreePath); removeErr != nil {
		slog.WarnContext(ctx, "cascade: worktree remove failed (continuing)",
			"ws", ws.ID, "worktree", ws.WorktreePath, "err", removeErr)
	}
	if ws.Branch != "" && ws.Branch == repo.DefaultBranch { //nolint:nestif // default-branch reattach vs force-delete teardown; the branch guard is load-bearing
		// The default branch is the unmanaged main folder's branch and the shared
		// integration branch — NEVER delete it on workspace removal. If the main
		// folder was detached to free it for this managed worktree, re-attach it
		// (removing the worktree above freed the branch); a no-op otherwise.
		if reErr := u.git.CheckoutBranch(ctx, repoPath, ws.Branch); reErr != nil {
			slog.WarnContext(ctx, "cascade: re-attach main folder to default branch (continuing)",
				"ws", ws.ID, "branch", ws.Branch, "err", reErr)
		}
	} else if delErr := u.git.ForceDeleteBranch(ctx, repoPath, ws.Branch); delErr != nil {
		slog.WarnContext(ctx, "cascade: branch delete failed (continuing)",
			"ws", ws.ID, "branch", ws.Branch, "err", delErr)
	}
	return u.workspaces.Delete(ctx, ws.ID)
}

// reapTerminals terminates every live PTY session owned by wsID. Best-effort and a
// no-op when no terminal reaper is wired (tests / virtual repos).
func (u *worktreeUsecase) reapTerminals(
	ctx context.Context,
	wsID string,
) {
	if u.terminals == nil {
		return
	}
	for _, sid := range u.terminals.ListSessionsForWorkspace(wsID) {
		if err := u.terminals.Kill(ctx, sid); err != nil {
			slog.WarnContext(ctx, "cascade: terminal kill failed (continuing)",
				"ws", wsID, "session", sid, "err", err)
		}
	}
}

func (u *worktreeUsecase) repoPathFor(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	repo, err := u.repos.FindByKey(ctx, ws.RepoID)
	if err != nil {
		return "", fmt.Errorf("repo path: %w", err)
	}
	if repo == nil {
		return "", fmt.Errorf("worktree: repo %s not found", ws.RepoID)
	}
	return repo.Path, nil
}

func (u *worktreeUsecase) childHasChildren(
	ctx context.Context,
	childID string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, err
	}
	for _, ws := range all {
		// A self-loop (ws.ParentID == ws.ID) must not count the node as its own
		// child, or a corrupted self-parented workspace becomes unreparentable.
		if ws.ParentID == childID && ws.ID != childID {
			return true, nil
		}
	}
	return false, nil
}

func nodesFrom(
	all []domain.Workspace,
) []cascade.Node {
	nodes := make([]cascade.Node, 0, len(all))
	for _, ws := range all {
		nodes = append(nodes, cascade.Node{
			ID:     ws.ID,
			Parent: ws.ParentID,
			Locked: ws.Status == domain.WorkspaceStatusLocked,
		})
	}
	return nodes
}

func indexByID(
	all []domain.Workspace,
) map[string]domain.Workspace {
	index := make(map[string]domain.Workspace, len(all))
	for _, ws := range all {
		index[ws.ID] = ws
	}
	return index
}
