// Package mocks provides hand-written fakes for the usecase collaborators
// (GORM stores, workspace repository, git and provider engines) used in tests.
package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchimport"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
	provider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// ProjectStore is a fake store.Store[domain.Project, string].
type ProjectStore struct {
	Saved   []domain.Project
	SaveErr error
	FindErr error
	// FindAllErr, when set, fails FindAll independently of FindErr (which only
	// guards FindByKey) — so a test can fail the list read without also
	// breaking every by-id lookup on the same store.
	FindAllErr error
}

// NewProjectStore returns an empty ProjectStore.
func NewProjectStore() *ProjectStore {
	return &ProjectStore{}
}

// Save UPSERTS by id, mirroring the real GORM store: re-saving a row replaces
// it rather than shadowing it with a duplicate the reads would then have to pick
// between. Anything that renumbers a list writes the same row more than once.
func (s *ProjectStore) Save(
	ctx context.Context,
	item domain.Project,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == item.ID {
			s.Saved[i] = item
			return nil
		}
	}
	s.Saved = append(s.Saved, item)
	return nil
}

// Delete removes the row so Saved reflects the net persisted state (a
// rollback after a Save leaves no row), mirroring RepositoryStore.Delete and
// the real GORM store.
func (s *ProjectStore) Delete(
	ctx context.Context,
	id string,
) error {
	kept := s.Saved[:0]
	for _, p := range s.Saved {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	s.Saved = kept
	return nil
}

func (s *ProjectStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Project, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			return &s.Saved[i], nil
		}
	}
	return nil, nil
}

func (s *ProjectStore) FindAll(
	ctx context.Context,
) ([]domain.Project, error) {
	if s.FindAllErr != nil {
		return nil, s.FindAllErr
	}
	return s.Saved, nil
}

// RepositoryStore is a fake store.Store[domain.Repository, string].
type RepositoryStore struct {
	Saved   []domain.Repository
	SaveErr error
	// FindErr, when set, fails FindAll — the read failure that callers such as
	// CheckRepoImportable deliberately degrade past rather than block on.
	FindErr error
	// FindByKeyErr, when set, fails FindByKey only — separate from FindErr so a
	// test can fail the single-row lookup without also breaking FindAll/FindWhere.
	FindByKeyErr error
	// SaveErrForID, when set for a given repo id, fails only that Save call —
	// separate from SaveErr so a test can fail one row's write (e.g. a sibling
	// renumbered by a densify pass) while another row's write in the same
	// operation still succeeds.
	SaveErrForID map[string]error
	// FindByKeyFn, when set, overrides the default lookup entirely — used when a
	// test needs to distinguish repeat FindByKey calls for the SAME id (e.g. the
	// load at the top of an update vs. the re-fetch after its Save) rather than
	// failing every call alike.
	FindByKeyFn func(id string) (*domain.Repository, error)
	// FindWhereFn, when set, overrides the default scoped query entirely — used
	// when a test needs one FindWhere call (e.g. densifying the ORIGIN project
	// after a cross-project repo move) to fail while another (the destination
	// project) succeeds, which a single blanket FindErr cannot express.
	FindWhereFn func(match domain.Repository) ([]domain.Repository, error)
}

// NewRepositoryStore returns an empty RepositoryStore.
func NewRepositoryStore() *RepositoryStore {
	return &RepositoryStore{}
}

// Save UPSERTS by id, mirroring the real GORM store: re-saving a row replaces
// it rather than shadowing it with a duplicate the reads would then have to pick
// between. Anything that renumbers a list writes the same row more than once.
func (s *RepositoryStore) Save(
	ctx context.Context,
	item domain.Repository,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	if err := s.SaveErrForID[item.ID]; err != nil {
		return err
	}
	for i := range s.Saved {
		if s.Saved[i].ID == item.ID {
			s.Saved[i] = item
			return nil
		}
	}
	s.Saved = append(s.Saved, item)
	return nil
}

func (s *RepositoryStore) Delete(
	ctx context.Context,
	id string,
) error {
	// Remove the row so Saved reflects the net persisted state (a rollback after a
	// Save leaves no row), matching the real store.
	kept := s.Saved[:0]
	for _, r := range s.Saved {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	s.Saved = kept
	return nil
}

func (s *RepositoryStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Repository, error) {
	if s.FindByKeyFn != nil {
		return s.FindByKeyFn(id)
	}
	if s.FindByKeyErr != nil {
		return nil, s.FindByKeyErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			return &s.Saved[i], nil
		}
	}
	return nil, nil
}

func (s *RepositoryStore) FindAll(
	ctx context.Context,
) ([]domain.Repository, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Saved, nil
}

// FindWhere mirrors the real store's prototype-scoped query: it matches on the
// non-zero fields of match, which for the repository table is the ProjectID the
// sidebar reorder narrows by.
func (s *RepositoryStore) FindWhere(
	ctx context.Context,
	match domain.Repository,
) ([]domain.Repository, error) {
	if s.FindWhereFn != nil {
		return s.FindWhereFn(match)
	}
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	rows := make([]domain.Repository, 0, len(s.Saved))
	for _, r := range s.Saved {
		if match.ProjectID != "" && r.ProjectID != match.ProjectID {
			continue
		}
		if match.ID != "" && r.ID != match.ID {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// WorkspacePlacements is a fake project.WorkspaceRelocator: it holds the
// workspace rows a repo move has to carry along, and records the writes made
// against them.
type WorkspacePlacements struct {
	Rows    []domain.Workspace
	ListErr error
	SetErr  error
}

// NewWorkspacePlacements returns an empty WorkspacePlacements.
func NewWorkspacePlacements() *WorkspacePlacements {
	return &WorkspacePlacements{}
}

func (s *WorkspacePlacements) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	rows := make([]domain.Workspace, 0, len(s.Rows))
	for _, w := range s.Rows {
		if w.ProjectID == projectID && w.RepoID == repoID {
			rows = append(rows, w)
		}
	}
	return rows, nil
}

func (s *WorkspacePlacements) SetProject(
	ctx context.Context,
	id string,
	projectID string,
) (domain.Workspace, error) {
	if s.SetErr != nil {
		return domain.Workspace{}, s.SetErr
	}
	for i := range s.Rows {
		if s.Rows[i].ID == id {
			s.Rows[i].ProjectID = projectID
			return s.Rows[i], nil
		}
	}
	return domain.Workspace{}, nil
}

// WorkspaceRepo is a fake of the subset of workspace.Workspace used on import.
type WorkspaceRepo struct {
	Created   []domain.Workspace
	CreateErr error
	// Deleted records the rows a rollback tombstoned, in order, so a test can
	// tell a workspace that was taken back out from one silently left behind.
	Deleted   []string
	DeleteErr error
	// CreateFn, when non-nil, is called instead of the default stub logic. The
	// caller is responsible for appending to Created if it wants tracking.
	CreateFn func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
}

// NewWorkspaceRepo returns an empty WorkspaceRepo.
func NewWorkspaceRepo() *WorkspaceRepo {
	return &WorkspaceRepo{}
}

func (r *WorkspaceRepo) Create(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	if r.CreateFn != nil {
		return r.CreateFn(ctx, in, now)
	}
	if r.CreateErr != nil {
		return domain.Workspace{}, r.CreateErr
	}
	status := domain.WorkspaceStatusNew
	if in.Protected {
		status = domain.WorkspaceStatusLocked
	}
	ws := domain.Workspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Status:        status,
		MergeStrategy: in.MergeStrategy,
		IsDefault:     in.IsDefault,
		Kind:          in.Kind,
		HeldByPath:    in.HeldByPath,
		CreatedAt:     now,
	}
	r.Created = append(r.Created, ws)
	return ws, nil
}

// Delete tombstones a created row, recording the id so a test can prove a
// workspace that could not be attached to its owning chat was taken back out
// rather than left orphaned.
func (r *WorkspaceRepo) Delete(
	_ context.Context,
	id string,
) error {
	if r.DeleteErr != nil {
		return r.DeleteErr
	}
	r.Deleted = append(r.Deleted, id)
	return nil
}

// GitEngine is a fake of the git operations the import usecase consumes.
type GitEngine struct {
	Worktrees       []gitengine.WorktreeEntry
	WorktreeListErr error
	MergeBaseSha    string
	MergeBaseErr    error

	// WorktreeListFn, when non-nil, overrides Worktrees/WorktreeListErr for
	// per-repo control in tests.
	WorktreeListFn func(repoPath string) ([]gitengine.WorktreeEntry, error)

	// Protected-branch managed-worktree provisioning fakes (project import).
	Detached     []string          // worktree paths detached to HEAD
	CheckedOut   []WorktreeAddCall // (path, branch) re-attach calls
	WorktreeAdds []WorktreeAddCall // (path, branch) worktrees materialised, by EITHER add
	//nolint:lll // the trailing note is the point: this log is the -B subset of WorktreeAdds.
	WorktreeAddAtRefs      []WorktreeAddAtRefCall // the subset added AT a start ref (`git worktree add -B`)
	WorktreeRemoves        []string               // worktree paths force-removed
	FetchedRefs            []string               // branches fetched from origin (FetchRef)
	FastForwardedBranches  []string               // branches fast-forwarded from origin (FastForwardBranch)
	RemoteBranches         map[string]bool        // branch -> exists on origin live (default false)
	RemoteTrackingBranches map[string]bool        // branch -> local refs/remotes/origin/<branch> present (default false)
	RevParseShas           map[string]string      // rev -> sha (default "")
	DetachErr              error                  // forces DetachWorktree to fail
	// WorktreeAddErrByBranch forces WorktreeAdd to fail for specific branches.
	WorktreeAddErrByBranch map[string]error
	// Pruned records repo paths WorktreePrune was called on.
	Pruned []string
	// DeadRegistrations maps a dead worktree dir -> branch it "holds"; WorktreePrune
	// removes them and merges the survivors into the list holder.Resolve sees.
	DeadRegistrations map[string]string
	// FastForwardErr forces FastForwardBranch to fail (best-effort FF path).
	FastForwardErr error
	// UpstreamsSet records branches linked to origin/<branch> via SetUpstream.
	UpstreamsSet []string
	// SetUpstreamErr forces SetUpstream to fail (best-effort tracking path).
	SetUpstreamErr error
	// WorktreeRemoveErr forces WorktreeRemove to fail (orphaned-worktree cleanup
	// after a failed workspace-row create).
	WorktreeRemoveErr error
	// RevParseErr forces RevParse to fail (the non-essential fork-point read
	// after a protected-branch worktree add).
	RevParseErr error
	// FetchRefErr forces FetchRef to fail (best-effort origin refresh before
	// checking a protected branch out at its origin ref).
	FetchRefErr error
}

// WorktreeAddCall records a fake WorktreeAdd invocation.
type WorktreeAddCall struct {
	Path   string
	Branch string
}

// WorktreeAddAtRefCall records a `git worktree add -B <branch> <startRef>`, so a
// test can assert the checkout started at ORIGIN's ref rather than at whatever
// the local branch pointed to.
type WorktreeAddAtRefCall struct {
	Path     string
	Branch   string
	StartRef string
}

// NewGitEngine returns an empty GitEngine.
func NewGitEngine() *GitEngine {
	return &GitEngine{}
}

func (g *GitEngine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]gitengine.WorktreeEntry, error) {
	if g.WorktreeListFn != nil {
		return g.WorktreeListFn(repoPath)
	}
	if g.WorktreeListErr != nil {
		return nil, g.WorktreeListErr
	}
	out := append([]gitengine.WorktreeEntry(nil), g.Worktrees...)
	for path, branch := range g.DeadRegistrations {
		out = append(out, gitengine.WorktreeEntry{Path: path, Branch: branch})
	}
	return out, nil
}

func (g *GitEngine) MergeBase(
	ctx context.Context,
	repoPath string,
	a string,
	b string,
) (string, error) {
	if g.MergeBaseErr != nil {
		return "", g.MergeBaseErr
	}
	return g.MergeBaseSha, nil
}

func (g *GitEngine) DetachWorktree(
	ctx context.Context,
	worktreePath string,
) error {
	if g.DetachErr != nil {
		return g.DetachErr
	}
	g.Detached = append(g.Detached, worktreePath)
	return nil
}

func (g *GitEngine) CheckoutBranch(
	ctx context.Context,
	worktreePath string,
	branch string,
) error {
	g.CheckedOut = append(g.CheckedOut, WorktreeAddCall{Path: worktreePath, Branch: branch})
	return nil
}

func (g *GitEngine) RemoteBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	return g.RemoteBranches[branch], nil
}

func (g *GitEngine) RemoteTrackingBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	return g.RemoteTrackingBranches[branch], nil
}

func (g *GitEngine) FetchRef(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	if g.FetchRefErr != nil {
		return g.FetchRefErr
	}
	g.FetchedRefs = append(g.FetchedRefs, branch)
	return nil
}

func (g *GitEngine) FastForwardBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	if g.FastForwardErr != nil {
		return g.FastForwardErr
	}
	g.FastForwardedBranches = append(g.FastForwardedBranches, branch)
	return nil
}

func (g *GitEngine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	if err := g.WorktreeAddErrByBranch[branch]; err != nil {
		return err
	}
	g.WorktreeAdds = append(g.WorktreeAdds, WorktreeAddCall{Path: worktreePath, Branch: branch})
	return nil
}

// WorktreeAddAtRef records into the SAME WorktreeAdds log as WorktreeAdd and
// honours the same per-branch error map: to a caller asserting "which branches
// got a worktree, and where", the two are one outcome — only the start point
// differs. Keeping them in one log is what lets the existing provisioning tests
// stay agnostic about which of the two the production path chose.
// SetUpstream records the branches linked back to origin/<branch>, so a test can
// assert the tracking info `git worktree add -B` does not create itself was set.
func (g *GitEngine) SetUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	g.UpstreamsSet = append(g.UpstreamsSet, branch)
	return g.SetUpstreamErr
}

func (g *GitEngine) WorktreeAddAtRef(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startRef string,
) (string, error) {
	if err := g.WorktreeAddErrByBranch[branch]; err != nil {
		return "", err
	}
	g.WorktreeAdds = append(g.WorktreeAdds, WorktreeAddCall{Path: worktreePath, Branch: branch})
	g.WorktreeAddAtRefs = append(g.WorktreeAddAtRefs, WorktreeAddAtRefCall{
		Path: worktreePath, Branch: branch, StartRef: startRef,
	})
	if sha, ok := g.RevParseShas[startRef]; ok {
		return sha, nil
	}
	return "", nil
}

func (g *GitEngine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	if g.WorktreeRemoveErr != nil {
		return g.WorktreeRemoveErr
	}
	g.WorktreeRemoves = append(g.WorktreeRemoves, worktreePath)
	return nil
}

func (g *GitEngine) WorktreePrune(
	ctx context.Context,
	repoPath string,
) error {
	g.Pruned = append(g.Pruned, repoPath)
	g.DeadRegistrations = nil // prune reaps every dead registration
	return nil
}

func (g *GitEngine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	if g.RevParseErr != nil {
		return "", g.RevParseErr
	}
	if sha, ok := g.RevParseShas[rev]; ok {
		return sha, nil
	}
	return "", nil
}

// ProviderEngine is a fake of the provider operations the import usecase consumes.
type ProviderEngine struct {
	Protected    []string
	ProtectedErr error
	AvatarURL    string
	AvatarURLErr error
	PRLinks      []provider.PRLink
	PRLinksErr   error
}

// NewProviderEngine returns an empty ProviderEngine.
func NewProviderEngine() *ProviderEngine {
	return &ProviderEngine{}
}

func (p *ProviderEngine) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	if p.ProtectedErr != nil {
		return nil, p.ProtectedErr
	}
	return p.Protected, nil
}

func (p *ProviderEngine) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	return p.AvatarURL, p.AvatarURLErr
}

func (p *ProviderEngine) OpenPullRequests(
	ctx context.Context,
	repoPath string,
) ([]provider.PRLink, error) {
	if p.PRLinksErr != nil {
		return nil, p.PRLinksErr
	}
	return p.PRLinks, nil
}

// ProviderSyncWorkspaceRepo is a fake of the workspace.Workspace surface used
// by ProviderSyncUsecase.
type ProviderSyncWorkspaceRepo struct {
	GetFn             func(ctx context.Context, id string) (domain.Workspace, error)
	SyncProviderFn    func(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
	ListFn            func(ctx context.Context) ([]domain.Workspace, error)
	SetParentFromPRFn func(ctx context.Context, id, parentID string) (domain.Workspace, error)
}

// NewProviderSyncWorkspaceRepo returns an empty ProviderSyncWorkspaceRepo.
func NewProviderSyncWorkspaceRepo() *ProviderSyncWorkspaceRepo {
	return &ProviderSyncWorkspaceRepo{}
}

func (r *ProviderSyncWorkspaceRepo) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return r.GetFn(ctx, id)
}

func (r *ProviderSyncWorkspaceRepo) SyncProviderState(
	ctx context.Context,
	in workspace.ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	return r.SyncProviderFn(ctx, in, now)
}

func (r *ProviderSyncWorkspaceRepo) List(ctx context.Context) ([]domain.Workspace, error) {
	if r.ListFn != nil {
		return r.ListFn(ctx)
	}
	return nil, nil
}

func (r *ProviderSyncWorkspaceRepo) SetParentFromPR(ctx context.Context, id, parentID string) (domain.Workspace, error) {
	if r.SetParentFromPRFn != nil {
		return r.SetParentFromPRFn(ctx, id, parentID)
	}
	return domain.Workspace{}, nil
}

// ProviderSyncEngine is a fake of the provider.Engine surface used by
// ProviderSyncUsecase.
type ProviderSyncEngine struct {
	PollOnViewFn func(ctx context.Context, wsID, repoPath, branch string) (provider.ProviderState, error)
}

// NewProviderSyncEngine returns an empty ProviderSyncEngine.
func NewProviderSyncEngine() *ProviderSyncEngine {
	return &ProviderSyncEngine{}
}

func (e *ProviderSyncEngine) PollOnView(
	ctx context.Context,
	wsID string,
	repoPath string,
	branch string,
) (provider.ProviderState, error) {
	return e.PollOnViewFn(ctx, wsID, repoPath, branch)
}

// WorkspaceLifecycleRepo is a fake of the workspace repo surface used by the
// workspace, file, and git usecases.
type WorkspaceLifecycleRepo struct {
	ListFn       func(ctx context.Context) ([]domain.Workspace, error)
	ListInRepoFn func(ctx context.Context, projectID, repoID string) ([]domain.Workspace, error)
	GetFn        func(ctx context.Context, id string) (domain.Workspace, error)

	SetLockFn func(
		ctx context.Context,
		id string,
		locked *bool,
		protected bool,
	) (domain.Workspace, error)

	SetMergeStrategyFn func(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	SyncWorkingTreeFn func(
		ctx context.Context,
		in workspace.SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflictsFn func(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// NewWorkspaceLifecycleRepo returns an empty WorkspaceLifecycleRepo.
func NewWorkspaceLifecycleRepo() *WorkspaceLifecycleRepo {
	return &WorkspaceLifecycleRepo{}
}

func (r *WorkspaceLifecycleRepo) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return r.ListFn(ctx)
}

func (r *WorkspaceLifecycleRepo) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	return r.ListInRepoFn(ctx, projectID, repoID)
}

func (r *WorkspaceLifecycleRepo) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return r.GetFn(ctx, id)
}

func (r *WorkspaceLifecycleRepo) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	return r.SetMergeStrategyFn(ctx, id, strategy)
}

// SetLock records the lock decision the usecase passed down, so a test can read
// back what it decided to persist. It answers with the row it was handed rather
// than a stored one — the usecase resolves the status itself via the command.
func (r *WorkspaceLifecycleRepo) SetLock(
	ctx context.Context,
	id string,
	locked *bool,
	protected bool,
) (domain.Workspace, error) {
	if r.SetLockFn != nil {
		return r.SetLockFn(ctx, id, locked, protected)
	}
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}

func (r *WorkspaceLifecycleRepo) SyncWorkingTreeState(
	ctx context.Context,
	in workspace.SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	return r.SyncWorkingTreeFn(ctx, in, now)
}

func (r *WorkspaceLifecycleRepo) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	if r.ResolveConflictsFn != nil {
		return r.ResolveConflictsFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}

// WorkingTreeGitEngine is a fake of the git WorkingTreeSummary surface.
type WorkingTreeGitEngine struct {
	WorkingTreeSummaryFn func(
		ctx context.Context,
		repoPath string,
		base string,
	) (int, int, bool, bool, error)
	WouldMergeConflictFn func(
		ctx context.Context,
		repoPath string,
		ours string,
		theirs string,
	) (bool, error)
	RevParseFn func(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
}

// NewWorkingTreeGitEngine returns an empty WorkingTreeGitEngine.
func NewWorkingTreeGitEngine() *WorkingTreeGitEngine {
	return &WorkingTreeGitEngine{}
}

func (g *WorkingTreeGitEngine) WorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	base string,
) (int, int, bool, bool, error) {
	return g.WorkingTreeSummaryFn(ctx, repoPath, base)
}

func (g *WorkingTreeGitEngine) WouldMergeConflict(
	ctx context.Context,
	repoPath string,
	ours string,
	theirs string,
) (bool, error) {
	if g.WouldMergeConflictFn == nil {
		return false, nil
	}
	return g.WouldMergeConflictFn(ctx, repoPath, ours, theirs)
}

// RevParse resolves rev, or reports it as resolvable by default so summaryBase's
// verification step is a no-op unless a test opts into RevParseFn to exercise the
// fork-point fallback.
func (g *WorkingTreeGitEngine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	if g.RevParseFn == nil {
		return rev, nil
	}
	return g.RevParseFn(ctx, repoPath, rev)
}

// ProjectRollup is a fake of the project lastActivity roll-up surface. It
// records the last repoID it was asked to touch.
type ProjectRollup struct {
	TouchedRepoID string
	Touched       bool
}

// NewProjectRollup returns an empty ProjectRollup.
func NewProjectRollup() *ProjectRollup {
	return &ProjectRollup{}
}

func (r *ProjectRollup) TouchProjectActivity(
	ctx context.Context,
	repoID string,
	now time.Time,
) {
	r.TouchedRepoID = repoID
	r.Touched = true
}

// WorkspaceSyncer is a fake of the workspace syncer surface used by the file and
// git usecases.
type WorkspaceSyncer struct {
	Synced     bool
	SyncedID   string
	SyncedIDs  []string
	Resolved   bool
	ResolvedID string

	GetFn     func(ctx context.Context, id string) (domain.Workspace, error)
	ListFn    func(ctx context.Context) ([]domain.Workspace, error)
	SyncFn    func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
	ResolveFn func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
}

// NewWorkspaceSyncer returns an empty WorkspaceSyncer.
func NewWorkspaceSyncer() *WorkspaceSyncer {
	return &WorkspaceSyncer{}
}

func (s *WorkspaceSyncer) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return s.GetFn(ctx, id)
}

// List returns the fake's workspace rows, defaulting to none when no ListFn is
// set (the cascade then finds no children — the pull/fetch resyncs only itself).
func (s *WorkspaceSyncer) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	if s.ListFn != nil {
		return s.ListFn(ctx)
	}
	return nil, nil
}

func (s *WorkspaceSyncer) SyncWorkingTreeState(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	s.Synced = true
	s.SyncedID = id
	s.SyncedIDs = append(s.SyncedIDs, id)
	if s.SyncFn != nil {
		return s.SyncFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}

func (s *WorkspaceSyncer) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	s.Resolved = true
	s.ResolvedID = id
	if s.ResolveFn != nil {
		return s.ResolveFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}

// FsEngine is a fake of the filesystem-engine surface used by the file usecase.
type FsEngine struct {
	TreeFn func(repoPath, dirPath string, provider file.FileStatusProvider) ([]domain.FileNode, error)

	ReadContentFn  func(repoPath, filePath string) (domain.FileContent, error)
	WriteContentFn func(repoPath, filePath, content, encoding string) error
	CreateFileFn   func(repoPath, filePath string) error
	CreateDirFn    func(repoPath, dirPath string) error
	CopyFn         func(repoPath, sourcePath, destPath string) error
	RenameFn       func(repoPath, oldPath, newPath string) error
	DeleteFn       func(repoPath, filePath string) error
}

// NewFsEngine returns an empty FsEngine.
func NewFsEngine() *FsEngine {
	return &FsEngine{}
}

func (e *FsEngine) Tree(
	repoPath string,
	dirPath string,
	provider file.FileStatusProvider,
) ([]domain.FileNode, error) {
	return e.TreeFn(repoPath, dirPath, provider)
}

func (e *FsEngine) ReadContent(
	repoPath string,
	filePath string,
) (domain.FileContent, error) {
	return e.ReadContentFn(repoPath, filePath)
}

func (e *FsEngine) WriteContent(
	repoPath string,
	filePath string,
	content string,
	encoding string,
) error {
	return e.WriteContentFn(repoPath, filePath, content, encoding)
}

func (e *FsEngine) CreateFile(
	repoPath string,
	filePath string,
) error {
	return e.CreateFileFn(repoPath, filePath)
}

func (e *FsEngine) CreateDir(
	repoPath string,
	dirPath string,
) error {
	return e.CreateDirFn(repoPath, dirPath)
}

func (e *FsEngine) Copy(
	repoPath string,
	sourcePath string,
	destPath string,
) error {
	return e.CopyFn(repoPath, sourcePath, destPath)
}

func (e *FsEngine) Rename(
	repoPath string,
	oldPath string,
	newPath string,
) error {
	return e.RenameFn(repoPath, oldPath, newPath)
}

func (e *FsEngine) Delete(
	repoPath string,
	filePath string,
) error {
	return e.DeleteFn(repoPath, filePath)
}

// TerminalEngine is a fake of the terminal-engine surface used by the terminal
// usecase. Create's first id is the OWNING CHAT; the directory it runs in is a
// separate lookup the usecase makes against the chat's worktree.
type TerminalEngine struct {
	CreateFn          func(ctx context.Context, chatID, dir string, prof *domain.TerminalProfile) (string, error)
	KillFn            func(ctx context.Context, sessionID string) error
	LoadPlaceholderFn func(ctx context.Context, m engineterminal.SessionMeta, scrollback []byte) error
}

// NewTerminalEngine returns an empty TerminalEngine.
func NewTerminalEngine() *TerminalEngine {
	return &TerminalEngine{}
}

func (e *TerminalEngine) Create(
	ctx context.Context,
	chatID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	return e.CreateFn(ctx, chatID, workspaceDir, prof)
}

func (e *TerminalEngine) Kill(
	ctx context.Context,
	sessionID string,
) error {
	return e.KillFn(ctx, sessionID)
}

// LoadPlaceholder is a no-op by default; set LoadPlaceholderFn to override.
func (e *TerminalEngine) LoadPlaceholder(
	ctx context.Context,
	m engineterminal.SessionMeta,
	scrollback []byte,
) error {
	if e.LoadPlaceholderFn != nil {
		return e.LoadPlaceholderFn(ctx, m, scrollback)
	}
	return nil
}

// TerminalProfileStore is a fake store.Store[domain.TerminalProfile, string].
type TerminalProfileStore struct {
	Saved   []domain.TerminalProfile
	Deleted []string

	SaveErr    error
	DeleteErr  error
	FindAllErr error
}

// NewTerminalProfileStore returns an empty TerminalProfileStore.
func NewTerminalProfileStore() *TerminalProfileStore {
	return &TerminalProfileStore{}
}

func (s *TerminalProfileStore) Save(
	ctx context.Context,
	item domain.TerminalProfile,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saved = append(s.Saved, item)
	return nil
}

func (s *TerminalProfileStore) Delete(
	ctx context.Context,
	id string,
) error {
	if s.DeleteErr != nil {
		return s.DeleteErr
	}
	s.Deleted = append(s.Deleted, id)
	return nil
}

func (s *TerminalProfileStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.TerminalProfile, error) {
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			return &s.Saved[i], nil
		}
	}
	return nil, nil
}

func (s *TerminalProfileStore) FindAll(
	ctx context.Context,
) ([]domain.TerminalProfile, error) {
	if s.FindAllErr != nil {
		return nil, s.FindAllErr
	}
	return s.Saved, nil
}

// AgentChatPlacements is a fake chat-tree Chats AND Agent:
// it holds every row the sidebar forest carries — folders and conversations
// alike, now the same aggregate — records the placement writes made against
// them, mints and starts the ones a create asks for, erases the ones a
// cascade purges or a folder delete forgets, and records the lineage notes a
// move asks for.
//
// Purged is kept in call order, because the cascade's whole contract is that a
// chat is erased only once every chat below it already has been.
//
// Noted is kept in call order too, and records the LINEAGE each note carried
// rather than merely that one happened: a note naming the wrong ancestors is
// worse than no note, since the record it writes into the chat's conversation is
// permanent and is what a reader would believe afterwards.
type AgentChatPlacements struct {
	Rows      []domain.Chat
	Purged    []string
	Forgotten []string
	Noted     []LineageNote
	Spawned   []string
	Minted    []string
	Started   []StartCall
	// SpawnedOwnWorktree records each SpawnChatWithOwnWorktree call, in the SAME
	// StartCall shape Started uses (including ParentAtStart) — CreateChat's
	// ownWorktree counterpart to Started above, and provable ordering for the
	// identical reason: the placement must land before this call, not after.
	SpawnedOwnWorktree []StartCall
	// SpawnedImportedWorktree records each SpawnChatWithImportedWorktree call in
	// the same shape, and ImportedSpecs the branch each one asked for — the
	// import counterpart of SpawnedOwnWorktree above.
	SpawnedImportedWorktree []StartCall
	ImportedBranches        []string
	// AttachedWorkspaces records each AttachWorkspace call as chatID->workspaceID,
	// in order, so a test can prove the chat existed before the workspace it owns.
	AttachedWorkspaces []PlacementWrite
	ListErr            error
	GetErr             error
	LoadErr            error
	SetErr             error
	OrderErr           error
	PurgeErr           error
	ForgetErr          error
	CreateErr          error
	TitleErr           error
	NoteErr            error
	SpawnErr           error
	MintErr            error
	StartErr           error
	// SpawnOwnWorktreeErr fails SpawnChatWithOwnWorktree, the same way StartErr
	// fails StartRunner.
	SpawnOwnWorktreeErr error
	// SpawnImportedWorktreeErr fails SpawnChatWithImportedWorktree, and
	// AttachWorkspaceErr fails AttachWorkspace, the same way StartErr fails
	// StartRunner.
	SpawnImportedWorktreeErr error
	AttachWorkspaceErr       error
	// ImportedWorkspace is the workspace SpawnChatWithImportedWorktree hands
	// back. Its zero value is an ordinary unlocked worktree; a test that wants
	// the row typed as a BRANCH sets Status to locked here.
	ImportedWorkspace domain.Workspace
	SetCalls          int
	MissingID         string
	// Placed and Ordered record the two placement writes SEPARATELY, because the
	// difference between them is the contract: a renumber may write an index and
	// must be unable to write a parent.
	Placed  []PlacementWrite
	Ordered []OrderWrite
	// Retyped records each SetType call, so a test can tell a row ADOPTED into a
	// new kind from a fresh row minted beside the old one — the two leave the
	// same type behind on the workspace and only this says which happened.
	Retyped []TypeWrite
	TypeErr error
	// Spoken names the rows something was actually SAID in, which is what tells
	// an owning row the backfill minted apart from somebody's conversation.
	Spoken   map[string]bool
	TurnsErr error
	// Stale is the PROJECTED placement of a chat — what ListByWorkspace, ListChats
	// and Get answer while the read model is behind the log. A placement write
	// returns as soon as the aggregate has it, so this is the daemon's ordinary
	// state for the microseconds after every one, not an exotic interleaving.
	Stale map[string]domain.Chat
	// NextID is the id the next MintChat hands back, so a test can name the chat a
	// create is about to make instead of discovering it from the return value.
	NextID string
}

// StartCall is one recorded call to StartRunner: the chat a CLI was started on,
// the provider it was started for, and — read AT THE MOMENT OF THE CALL — where
// that chat was sitting in the tree.
//
// ParentAtStart is the whole point of recording anything here. A create that
// placed the chat AFTER starting its CLI produces exactly the same end state as
// one that placed it before: same chat, same parent, same runner. The only thing
// that tells them apart is where the row was when the CLI came up, because that is
// what the spawn read to decide what the new chat inherits.
type StartCall struct {
	ChatID        string
	ProviderID    string
	ParentAtStart string
}

// PlacementWrite is one recorded call to SetPlacement: a row the caller MOVED,
// and where to.
type PlacementWrite struct {
	ChatID   string
	ParentID string
	Order    int
}

// OrderWrite is one recorded call to SetOrder: a row a densify renumbered, and
// the index it was given. It carries no parent, which is the whole point.
type OrderWrite struct {
	ChatID string
	Order  int
}

// TypeWrite is one recorded call to SetType: the row whose kind was rewritten,
// and what it became.
type TypeWrite struct {
	ChatID string
	Type   domain.ChatType
}

// LineageNote is one recorded call to NoteThreadLineage: which chat was told,
// and what it was told it now reads.
type LineageNote struct {
	ChatID    string
	Ancestors []string
}

// NewAgentChatPlacements returns an empty AgentChatPlacements.
func NewAgentChatPlacements() *AgentChatPlacements {
	return &AgentChatPlacements{}
}

func (s *AgentChatPlacements) ListByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	rows := make([]domain.Chat, 0, len(s.Rows))
	for _, c := range s.Rows {
		if c.WorkspaceID == workspaceID && c.ID != s.MissingID {
			rows = append(rows, s.projected(c))
		}
	}
	return rows, nil
}

// ListChats answers every row the daemon knows — folders included, since they
// are the same aggregate now — the read folder CRUD plans against.
func (s *AgentChatPlacements) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	rows := make([]domain.Chat, 0, len(s.Rows))
	for _, c := range s.Rows {
		if c.ID != s.MissingID {
			rows = append(rows, s.projected(c))
		}
	}
	return rows, nil
}

// projected answers with the row as the READ MODEL has it, which is the row
// itself unless a test is holding the projection behind the log.
func (s *AgentChatPlacements) projected(
	row domain.Chat,
) domain.Chat {
	stale, ok := s.Stale[row.ID]
	if !ok {
		return row
	}
	return stale
}

// Get is GetChat under the tree usecase's shorter name — see EventStore.Get.
func (s *AgentChatPlacements) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return s.GetChat(ctx, id)
}

func (s *AgentChatPlacements) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	if s.GetErr != nil {
		return domain.Chat{}, s.GetErr
	}
	for _, c := range s.Rows {
		if c.ID == id {
			return s.projected(c), nil
		}
	}
	return domain.Chat{}, apperr.ErrNotFound
}

// LoadChat is the log fold: it answers with the row as it actually stands,
// whatever the projection is still serving.
func (s *AgentChatPlacements) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	if s.LoadErr != nil {
		return domain.Chat{}, s.LoadErr
	}
	for _, c := range s.Rows {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.Chat{}, apperr.ErrNotFound
}

func (s *AgentChatPlacements) SetPlacement(
	ctx context.Context,
	chatID string,
	parentID string,
	order int,
) (domain.Chat, error) {
	s.SetCalls++
	if s.SetErr != nil {
		return domain.Chat{}, s.SetErr
	}
	s.Placed = append(s.Placed, PlacementWrite{ChatID: chatID, ParentID: parentID, Order: order})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].ParentID = parentID
			s.Rows[i].Order = order
			return s.Rows[i], nil
		}
	}
	return domain.Chat{}, nil
}

// SetOrder writes the index and leaves the parent exactly as it stands, like the
// command it stands in for: the aggregate applies it to the row folded from the
// log, so there is no parent in the call to be stale.
func (s *AgentChatPlacements) SetOrder(
	ctx context.Context,
	chatID string,
	order int,
) (domain.Chat, error) {
	if s.OrderErr != nil {
		return domain.Chat{}, s.OrderErr
	}
	s.Ordered = append(s.Ordered, OrderWrite{ChatID: chatID, Order: order})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].Order = order
			return s.Rows[i], nil
		}
	}
	return domain.Chat{}, nil
}

// SetType rewrites a row's kind and touches nothing else, like the command it
// stands in for. Retyped records the calls so a test can prove a workspace's
// row was ADOPTED rather than replaced.
func (s *AgentChatPlacements) SetType(
	ctx context.Context,
	chatID string,
	chatType domain.ChatType,
) (domain.Chat, error) {
	if s.TypeErr != nil {
		return domain.Chat{}, s.TypeErr
	}
	s.Retyped = append(s.Retyped, TypeWrite{ChatID: chatID, Type: chatType})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].Type = chatType
			return s.Rows[i], nil
		}
	}
	return domain.Chat{}, apperr.ErrNotFound
}

// Create mints a bare, unplaced row — the panel-root folder MintChat already
// mints a bare, unplaced chat as.
func (s *AgentChatPlacements) Create(
	ctx context.Context,
	in agentchat.CreateInput,
) (domain.Chat, error) {
	if s.CreateErr != nil {
		return domain.Chat{}, s.CreateErr
	}
	row := domain.Chat{ID: in.ID, Type: in.Type, WorkspaceID: in.WorkspaceID, CreatedAt: in.Now}
	s.Rows = append(s.Rows, row)
	return row, nil
}

func (s *AgentChatPlacements) SetTitle(
	ctx context.Context,
	chatID string,
	title string,
	source string,
) (domain.Chat, error) {
	if s.TitleErr != nil {
		return domain.Chat{}, s.TitleErr
	}
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].Title = title
			return s.Rows[i], nil
		}
	}
	return domain.Chat{}, apperr.ErrNotFound
}

// Forget erases a row outright — what a folder delete calls instead of
// PurgeChat, since a folder never had a runner or a ledger to tear down.
func (s *AgentChatPlacements) Forget(
	ctx context.Context,
	id string,
) error {
	if s.ForgetErr != nil {
		return s.ForgetErr
	}
	s.Forgotten = append(s.Forgotten, id)
	kept := s.Rows[:0]
	for _, c := range s.Rows {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	s.Rows = kept
	return nil
}

func (s *AgentChatPlacements) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	if s.PurgeErr != nil {
		return s.PurgeErr
	}
	s.Purged = append(s.Purged, chatID)
	kept := s.Rows[:0]
	for _, c := range s.Rows {
		if c.ID != chatID {
			kept = append(kept, c)
		}
	}
	s.Rows = kept
	return nil
}

// HasTurns answers whether anything was said in a chat. A row no test named in
// Spoken answers false — the state a row the backfill just minted is in.
func (s *AgentChatPlacements) HasTurns(
	ctx context.Context,
	chatID string,
) (bool, error) {
	if s.TurnsErr != nil {
		return false, s.TurnsErr
	}
	return s.Spoken[chatID], nil
}

func (s *AgentChatPlacements) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	if s.NoteErr != nil {
		return s.NoteErr
	}
	s.Noted = append(s.Noted, LineageNote{ChatID: chatID, Ancestors: ancestors})
	return nil
}

func (s *AgentChatPlacements) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (string, string, error) {
	if s.SpawnErr != nil {
		return "", "", s.SpawnErr
	}
	s.Spawned = append(s.Spawned, providerID)
	id := s.NextID
	s.Rows = append(s.Rows, domain.Chat{ID: id, Type: domain.ChatTypeChat, WorkspaceID: workspaceID})
	return id, "runner-" + id, nil
}

func (s *AgentChatPlacements) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	if s.MintErr != nil {
		return "", s.MintErr
	}
	s.Minted = append(s.Minted, workspaceID)
	s.Rows = append(s.Rows, domain.Chat{ID: s.NextID, Type: domain.ChatTypeChat, WorkspaceID: workspaceID})
	return s.NextID, nil
}

func (s *AgentChatPlacements) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	if s.StartErr != nil {
		return "", s.StartErr
	}
	s.Started = append(s.Started, StartCall{
		ChatID:        chatID,
		ProviderID:    providerID,
		ParentAtStart: s.parentOf(chatID),
	})
	return "runner-" + chatID, nil
}

// SpawnChatWithOwnWorktree fakes agentusecase.Usecase.SpawnChatWithOwnWorktree:
// it records the call and, on success, fills the row's WorkspaceID — as the
// real one does via WorktreeCreator.CreateChildWorkspace + Chats.SetWorkspace —
// so a test can assert the row actually ended up owning a workspace, not merely
// that the call happened.
func (s *AgentChatPlacements) SpawnChatWithOwnWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	if s.SpawnOwnWorktreeErr != nil {
		return "", s.SpawnOwnWorktreeErr
	}
	s.SpawnedOwnWorktree = append(s.SpawnedOwnWorktree, StartCall{
		ChatID:        chatID,
		ProviderID:    providerID,
		ParentAtStart: s.parentOf(chatID),
	})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = "ws-child-" + chatID
		}
	}
	return "runner-" + chatID, nil
}

// SpawnChatWithImportedWorktree mirrors SpawnChatWithOwnWorktree for an
// existing branch: it records the call and the branch, fills the row's
// workspace slot, and hands back the workspace so the caller can decide what
// kind of row the chat is. An empty providerID starts no runner, exactly as
// production does.
func (s *AgentChatPlacements) SpawnChatWithImportedWorktree(
	_ context.Context,
	chatID string,
	providerID string,
	spec branchimport.Spec,
) (domain.Workspace, string, error) {
	if s.SpawnImportedWorktreeErr != nil {
		return domain.Workspace{}, "", s.SpawnImportedWorktreeErr
	}
	s.SpawnedImportedWorktree = append(s.SpawnedImportedWorktree, StartCall{
		ChatID:        chatID,
		ProviderID:    providerID,
		ParentAtStart: s.parentOf(chatID),
	})
	s.ImportedBranches = append(s.ImportedBranches, spec.Branch)
	ws := s.ImportedWorkspace
	if ws.ID == "" {
		ws.ID = "ws-import-" + chatID
	}
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = ws.ID
		}
	}
	if providerID == "" {
		return ws, "", nil
	}
	return ws, "runner-" + chatID, nil
}

// AttachWorkspace records the slot write and applies it to the held row.
func (s *AgentChatPlacements) AttachWorkspace(
	_ context.Context,
	chatID string,
	workspaceID string,
) error {
	if s.AttachWorkspaceErr != nil {
		return s.AttachWorkspaceErr
	}
	s.AttachedWorkspaces = append(s.AttachedWorkspaces, PlacementWrite{
		ChatID:   chatID,
		ParentID: workspaceID,
	})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = workspaceID
		}
	}
	return nil
}

func (s *AgentChatPlacements) parentOf(
	chatID string,
) string {
	for _, c := range s.Rows {
		if c.ID == chatID {
			return c.ParentID
		}
	}
	return ""
}

// AgentWorkspaceGitStatus fakes the chat tree usecase's WorkspaceGitStatus
// seam: each workspace's already-synced Added/Deleted counts, keyed by
// workspace id, with no live git call behind it.
type AgentWorkspaceGitStatus struct {
	Summaries map[string][2]int
	Err       error
}

// NewAgentWorkspaceGitStatus returns an AgentWorkspaceGitStatus with no
// workspace summaries recorded.
func NewAgentWorkspaceGitStatus() *AgentWorkspaceGitStatus {
	return &AgentWorkspaceGitStatus{Summaries: map[string][2]int{}}
}

// Set records workspaceID's Added/Deleted for WorkingTreeSummary to answer
// with. A workspace never Set here answers 0, 0 — the zero value a workspace
// with a clean working tree would also report.
func (s *AgentWorkspaceGitStatus) Set(
	workspaceID string,
	added int,
	deleted int,
) {
	s.Summaries[workspaceID] = [2]int{added, deleted}
}

func (s *AgentWorkspaceGitStatus) WorkingTreeSummary(
	ctx context.Context,
	workspaceID string,
) (int, int, error) {
	if s.Err != nil {
		return 0, 0, s.Err
	}
	pair := s.Summaries[workspaceID]
	return pair[0], pair[1], nil
}

// AgentWorkspaceRoster fakes the chat tree usecase's WorkspaceRoster seam: the
// daemon's whole workspace census, answered in the order it was seeded — which
// is deliberately NOT sorted, since the backfill's own root-first ordering is
// the thing under test.
type AgentWorkspaceRoster struct {
	Rows []domain.Workspace
	Err  error
}

// NewAgentWorkspaceRoster returns a roster holding no workspaces.
func NewAgentWorkspaceRoster() *AgentWorkspaceRoster {
	return &AgentWorkspaceRoster{}
}

func (s *AgentWorkspaceRoster) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Rows, nil
}

// AgentWorkspaceReaper fakes the chat tree usecase's WorkspaceReaper seam: the
// worktree teardown a cascading chat delete puts each workspace in its subtree
// through.
//
// Reaped records every workspace id torn down, IN ORDER, because the order is
// part of the contract under test: the subtree is walked deepest first so a
// child's worktree goes before its lineage parent's cascade could reach it.
//
// Err fails EVERY reap; ErrFor fails one named workspace, which is what a test
// needs to prove the failure contract — a delete that cannot tear a worktree
// down must leave the chat, and its workspace, entirely intact.
// Roster, when set, is the workspace census this reaper actually REMOVES from.
// It is what lets a test assert the workspace is gone rather than merely that a
// method was called — the difference between proving the bug is fixed and
// proving a line of code runs.
type AgentWorkspaceReaper struct {
	Reaped []string
	Roster *AgentWorkspaceRoster
	Err    error
	ErrFor map[string]error
}

// NewAgentWorkspaceReaper returns a reaper that tears everything down happily.
func NewAgentWorkspaceReaper() *AgentWorkspaceReaper {
	return &AgentWorkspaceReaper{ErrFor: map[string]error{}}
}

// NewAgentWorkspaceReaperOver returns a reaper that removes what it reaps from
// roster, so "is it gone?" is a question about the census rather than about the
// call log.
func NewAgentWorkspaceReaperOver(
	roster *AgentWorkspaceRoster,
) *AgentWorkspaceReaper {
	return &AgentWorkspaceReaper{ErrFor: map[string]error{}, Roster: roster}
}

func (s *AgentWorkspaceReaper) DiscardChildWorkspace(
	ctx context.Context,
	workspaceID string,
) error {
	if err, ok := s.ErrFor[workspaceID]; ok {
		return err
	}
	if s.Err != nil {
		return s.Err
	}
	s.Reaped = append(s.Reaped, workspaceID)
	if s.Roster != nil {
		kept := make([]domain.Workspace, 0, len(s.Roster.Rows))
		for _, w := range s.Roster.Rows {
			if w.ID != workspaceID {
				kept = append(kept, w)
			}
		}
		s.Roster.Rows = kept
	}
	return nil
}
