package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// CreateChildInput, ImportInput and MergeResult are the hierarchy usecase's
// own value types, re-exported here so a caller of this package's public face
// never has to import internal/hierarchy directly.
type (
	CreateChildInput = hierarchy.CreateChildInput
	ImportInput      = hierarchy.ImportInput
	MergeResult      = hierarchy.MergeResult
	Option           = hierarchy.Option
	TerminalReaper   = hierarchy.TerminalReaper
	ChatWorkObserver = hierarchy.ChatWorkObserver
)

// WithTerminalReaper wires the terminal engine so cascade delete can kill a
// workspace's live PTY sessions. Without it, terminal cleanup is skipped.
var WithTerminalReaper = hierarchy.WithTerminalReaper

// The hierarchy usecase's sentinel errors, re-exported here for the same
// reason as the value types above.
var (
	ErrParentLocked                 = hierarchy.ErrParentLocked
	ErrRebaseNonLeaf                = hierarchy.ErrRebaseNonLeaf
	ErrChildHasChildren             = hierarchy.ErrChildHasChildren
	ErrCrossRepoWorktreeMove        = hierarchy.ErrCrossRepoWorktreeMove
	ErrSelfParent                   = hierarchy.ErrSelfParent
	ErrWorkspaceLocked              = hierarchy.ErrWorkspaceLocked
	ErrBranchWorkspaceExists        = hierarchy.ErrBranchWorkspaceExists
	ErrParentUnprovisioned          = hierarchy.ErrParentUnprovisioned
	ErrRenameUnmanagedWorkspace     = hierarchy.ErrRenameUnmanagedWorkspace
	ErrRenameTargetExists           = hierarchy.ErrRenameTargetExists
	ErrBranchStillHeld              = hierarchy.ErrBranchStillHeld
	ErrBranchHeldByManagedWorkspace = hierarchy.ErrBranchHeldByManagedWorkspace
	ErrWorkspaceWorking             = hierarchy.ErrWorkspaceWorking
)

// WorkspaceLifecycleRepo is the workspace repository surface the usecase needs.
type WorkspaceLifecycleRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
		protected bool,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in wsrepo.SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// WorkingTreeGitEngine is the git surface used to recompute the summary and to
// predict whether folding a child into its parent would conflict.
type WorkingTreeGitEngine interface {
	WorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		base string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)
	WouldMergeConflict(
		ctx context.Context,
		repoPath string,
		ours string,
		theirs string,
	) (bool, error)
	// RevParse resolves a ref to a commit SHA; summaryBase uses it only to verify
	// a base branch still resolves in the worktree before diffing against it.
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
}

// ProjectActivityRollup is the best-effort project lastActivity roll-up surface.
type ProjectActivityRollup interface {
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)
}

// Usecase is the workspace feature's one public face: the lifecycle/read
// surface over the workspace aggregate PLUS the worktree hierarchy (07) —
// worktree-backed create, local child→parent merge, re-parenting, and cascade
// delete. The hierarchy methods (CreateChild through SetChatObserver) are a
// thin delegation onto internal/hierarchy, which holds their implementation;
// this interface only names the one surface a caller sees.
// SyncWorkingTreeState is the shared wrapper the watcher, file, and git usecases
// call to recompute and persist the working-tree summary (00 §5.3).
type Usecase interface {
	// List returns every workspace row from the read model.
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)

	// ListInRepo returns every workspace row scoped to one project+repo, reading
	// from the same per-entity stores as List (see workspace.Workspace.ListInRepo)
	// — cheaper because it skips entities outside the requested repo, not because
	// it uses a different data source.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)

	// Get returns the workspace with the given id.
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)

	// SetMergeStrategy writes the mergeStrategy field (used by review PATCH).
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)

	// SetLock records the user's own lock decision for a workspace, which
	// outranks the provider's protected flag from here on. `locked` nil hands the
	// question back to the provider.
	//
	// Automatic locking is untouched by this: a protected branch is still created
	// locked and still re-locked by every provider poll. What the override adds is
	// the ability to disagree — to unlock main, or to lock a fork child the
	// provider has no opinion about — and to have that disagreement survive the
	// next poll.
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
	) (domain.Workspace, error)

	// SyncWorkingTreeState recomputes the working-tree summary from git, issues
	// the sync command, and rolls up project lastActivity best-effort.
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	// ResolveConflicts clears a sticky pr-conflicts status once the operation in
	// the workspace's own worktree has been resolved (the git usecase calls this
	// on a successful operation continue).
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	// MergeEligibilityFor resolves whether ws can be merged into its local
	// parent, reading the parent's status from the caller-held sibling set. The
	// ctx scopes the predicted-conflict git dry-run.
	MergeEligibilityFor(
		ctx context.Context,
		ws domain.Workspace,
		siblings []domain.Workspace,
	) MergeEligibility

	// CreateChild creates a worktree-backed (or workspace-only) child (07 §4.1).
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	// CreateFromImport batch-imports branches as managed workspaces, PR-parented
	// up to a protected/default root, creating missing ancestors (07 §import).
	CreateFromImport(
		ctx context.Context,
		in ImportInput,
	) error
	// MergeIntoParent runs a local child→parent merge under one of the three
	// strategies (07 §3.1).
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	// Reparent rebases a leaf child onto a new parent's tip (07 §4).
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	// RenameBranch renames a managed workspace's branch and relocates its
	// workspace root to the directory the new name derives.
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
	// RebaseOntoParent is the user-initiated "finish the move" action for a
	// moved-but-conflicting child.
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	// RetryProvision re-runs holder resolution + provisioning for an existing
	// placeholder (spec §3.3).
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees a live holder off the placeholder's branch, then
	// Retry-provisions in place (spec §3.5/§3.7).
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DeleteCascade removes rootID and its unlocked descendants (07 §5).
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
	// DeleteRepoWorkspaces removes every workspace of a repo, taking the repo's
	// path from the caller rather than resolving it from the (possibly
	// already-deleted) repo row.
	DeleteRepoWorkspaces(
		ctx context.Context,
		repoID string,
		repoPath string,
	) ([]string, error)
	// SetChatObserver wires the chat-usecase surface guardReparent's
	// working-chat check needs (invariant 5). It is a post-construction setter
	// because the chat usecase itself depends on this one (Promote forks a
	// workspace through it) — see hierarchy.Usecase.SetChatObserver.
	SetChatObserver(observer ChatWorkObserver)
	// SetOwningChatReconciler wires the chat-tree surface SetLock notifies when
	// a workspace becomes locked. Like SetChatObserver it is a post-construction
	// setter, and for the same reason: the chat side depends on this usecase, so
	// it does not exist yet at this one's own call site.
	SetOwningChatReconciler(reconciler OwningChatReconciler)
}

// OwningChatReconciler brings one workspace's owning chat row into line with
// what that workspace now IS.
//
// A workspace's owning row is typed for its character — a locked branch owns a
// BRANCH row, an open worktree owns a chat row — and that character can change
// at runtime, long after the row was made. The boot pass reconciles every
// workspace at startup; this is what closes the gap in between, so a branch
// locked at 3pm has its branch row at 3pm and not at the next restart.
type OwningChatReconciler interface {
	EnsureOwningChat(
		ctx context.Context,
		ws domain.Workspace,
	) error
}

type workspaceUsecase struct {
	repo   WorkspaceLifecycleRepo
	git    WorkingTreeGitEngine
	rollup ProjectActivityRollup
	// owningChats is notified when SetLock locks a workspace. Nil until the
	// composition root wires it, and a nil one simply reconciles nothing — the
	// next boot's pass still catches up.
	owningChats OwningChatReconciler
	hierarchy.Usecase
}

// SetOwningChatReconciler implements Usecase.
func (u *workspaceUsecase) SetOwningChatReconciler(
	reconciler OwningChatReconciler,
) {
	u.owningChats = reconciler
}

// New builds a Usecase from the workspace repo, the git engine, and the
// project roll-up usecase — the lifecycle methods' own three dependencies,
// unchanged from before this package absorbed the worktree hierarchy — plus
// everything the hierarchy needs beyond those. hierarchyWorkspaces and
// hierarchyGit are the SAME repository and git engine as repo/git above,
// widened to the fuller interfaces the hierarchy's own create/merge/reparent
// logic uses (repo/git only need the narrow slice the lifecycle methods call);
// they are separate parameters, not one, so a caller exercising only the
// lifecycle half (see workspace_test.go) can keep supplying its existing
// narrow fakes without also having to satisfy the wider ones. provider,
// repos, now and crowbarHome are the hierarchy's remaining dependencies
// verbatim (repos resolves a workspace's repository main path so worktree
// removal and branch deletion run against the repo, never a child's own
// worktree); opts configures it further (e.g. WithTerminalReaper).
func New(
	repo WorkspaceLifecycleRepo,
	git WorkingTreeGitEngine,
	rollup ProjectActivityRollup,
	hierarchyWorkspaces wsrepo.Workspace,
	hierarchyGit enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
	opts ...Option,
) Usecase {
	return &workspaceUsecase{
		repo:    repo,
		git:     git,
		rollup:  rollup,
		Usecase: hierarchy.New(hierarchyWorkspaces, hierarchyGit, provider, repos, now, crowbarHome, opts...),
	}
}

// List returns every workspace row from the read model.
func (u *workspaceUsecase) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	list, err := u.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return list, nil
}

// ListInRepo returns every workspace row scoped to one project+repo, reading
// from the same per-entity stores as List (see workspace.Workspace.ListInRepo)
// — cheaper because it skips entities outside the requested repo, not because
// it uses a different data source.
func (u *workspaceUsecase) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	list, err := u.repo.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list in repo: %w", err)
	}
	return list, nil
}

// Get returns the workspace with the given id.
func (u *workspaceUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return ws, nil
}

// SetMergeStrategy writes the mergeStrategy field (used by review PATCH).
func (u *workspaceUsecase) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	ws, err := u.repo.SetMergeStrategy(ctx, id, strategy)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return ws, nil
}

// SetLock records the user's own lock decision for a workspace.
//
// The provider's current protected answer is read off the stored row rather than
// polled: it is only needed to resolve the status when the override is being
// CLEARED, and a workspace whose status is locked while it carries no override
// is locked precisely because the provider said so.
func (u *workspaceUsecase) SetLock(
	ctx context.Context,
	id string,
	locked *bool,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set lock: get: %w", err)
	}
	protected := ws.LockOverride == nil && ws.Status == domain.WorkspaceStatusLocked
	updated, err := u.repo.SetLock(ctx, id, locked, protected)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set lock: %w", err)
	}
	u.reconcileOwningChat(ctx, ws.Status, updated)
	return updated, nil
}

// reconcileOwningChat gives a workspace that has JUST become locked the branch
// row it is owed, there and then.
//
// It fires on the TRANSITION and not on the state, so re-locking an already
// locked workspace costs nothing. Only the lock direction is watched: what a
// workspace is besides locked — the repo home, the project home — is fixed at
// creation and cannot change under a write.
//
// Synchronous, so the row exists before the caller is told the lock succeeded:
// every reader downstream of that answer is entitled to assume a locked
// workspace has a branch row, and a reconcile racing the response would hand
// them a window in which it does not.
//
// A failure is logged, never returned. The lock itself has already been
// committed and stands, so failing the call would report an error for a change
// that happened — and the boot pass reconciles the same workspace on the next
// start regardless.
func (u *workspaceUsecase) reconcileOwningChat(
	ctx context.Context,
	before domain.WorkspaceStatus,
	after domain.Workspace,
) {
	if u.owningChats == nil ||
		after.Status != domain.WorkspaceStatusLocked ||
		before == domain.WorkspaceStatusLocked {
		return
	}
	if err := u.owningChats.EnsureOwningChat(ctx, after); err != nil {
		slog.WarnContext(ctx, "workspace: set lock: reconcile the owning chat row",
			"workspace_id", after.ID, "err", err)
	}
}

// SyncWorkingTreeState recomputes the working-tree summary from git, issues the
// sync command, and rolls up project lastActivity best-effort.
func (u *workspaceUsecase) SyncWorkingTreeState(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync working tree: get: %w", err)
	}
	in, err := u.summarize(ctx, ws)
	if err != nil {
		return domain.Workspace{}, err
	}
	synced, err := u.repo.SyncWorkingTreeState(ctx, in, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync working tree: %w", err)
	}
	u.rollup.TouchProjectActivity(ctx, ws.RepoID, now)
	return synced, nil
}

// ResolveConflicts clears the workspace's pr-conflicts status after the operation
// in its own worktree has been resolved (the git usecase calls this on a
// successful operation continue). pr-conflicts is sticky otherwise, so this is
// the path that lets a resolved kept-rebase drop its conflict warning.
func (u *workspaceUsecase) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	ws, err := u.repo.ResolveConflicts(ctx, id, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: resolve conflicts: %w", err)
	}
	u.rollup.TouchProjectActivity(ctx, ws.RepoID, now)
	return ws, nil
}

// MergeEligibilityFor resolves whether ws can be merged into its local parent.
// No repository call is made — the parent is resolved from siblings, which the
// caller already holds from a preceding List call. Delegates to
// ResolveMergeEligibility so the snapshot read and the live broadcast share one
// implementation; the caller supplies the context for the conflict dry-run.
func (u *workspaceUsecase) MergeEligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
	siblings []domain.Workspace,
) MergeEligibility {
	return ResolveMergeEligibility(ctx, ws, siblings, u.git)
}

func (u *workspaceUsecase) summarize(
	ctx context.Context,
	ws domain.Workspace,
) (wsrepo.SyncInput, error) {
	// The project-level home workspace is rooted at the PROJECT directory — the
	// folder that contains repos, which is deliberately not a git repository (its
	// route group mounts no git surface at all). Shelling out to git there exits
	// 129 "Not a git repository", and because the summary is the first step of the
	// post-mutation resync, that failure propagated all the way out as a 500 on
	// every home file write: the bytes landed on disk but the editor reported
	// "Failed to save file" and kept the buffer dirty. A home workspace has no
	// branch to diff and no index to conflict, so its summary is zero by
	// definition — return it without touching git.
	if ws.Kind == domain.WorkspaceKindHome {
		return wsrepo.SyncInput{ID: ws.ID}, nil
	}
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(
		ctx,
		ws.WorktreePath,
		u.summaryBase(ctx, ws),
	)
	if err != nil {
		return wsrepo.SyncInput{}, fmt.Errorf("workspace: sync working tree: summary: %w", err)
	}
	return wsrepo.SyncInput{
		ID:           ws.ID,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	}, nil
}

// summaryBase returns the ref the working-tree summary diffs against: the base
// BRANCH NAME — the parent's branch for a child, or the workspace's own branch
// for a protected root — so the engine measures against the merge-base of that
// branch's current tip and HEAD and the sidebar diff self-corrects as the base
// branch advances (the frozen ForkPointSha inflates both children AND roots once
// their recorded fork point falls behind, e.g. a develop root left at an old tip).
//
// It falls back to the recorded ForkPointSha whenever the base branch is unusable:
// no branch name (a detached home), an unresolvable parent row, or a branch that
// no longer resolves in the worktree (renamed/deleted out of band). Verifying
// resolvability here — rather than letting the engine diff against a name that
// yields no merge-base and silently reports +0/-0 — keeps the summary consistent
// with the branch-review pane, which has the same fork-point fallback.
func (u *workspaceUsecase) summaryBase(
	ctx context.Context,
	ws domain.Workspace,
) string {
	base := ws.Branch
	if ws.ParentID != "" {
		parent, err := u.repo.Get(ctx, ws.ParentID)
		if err != nil {
			return ws.ForkPointSha
		}
		base = parent.Branch
	}
	if base == "" {
		return ws.ForkPointSha
	}
	if _, err := u.git.RevParse(ctx, ws.WorktreePath, base); err != nil {
		return ws.ForkPointSha
	}
	return base
}
