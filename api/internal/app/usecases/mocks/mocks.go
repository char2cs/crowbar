// Package mocks provides hand-written fakes for the usecase collaborators
// (GORM stores, workspace repository, git and provider engines) used in tests.
package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
	provider "github.com/char2cs/crowbar/api/internal/engine/provider"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// ProjectStore is a fake store.Store[domain.Project, string].
type ProjectStore struct {
	Saved   []domain.Project
	SaveErr error
	FindErr error
}

// NewProjectStore returns an empty ProjectStore.
func NewProjectStore() *ProjectStore {
	return &ProjectStore{}
}

func (s *ProjectStore) Save(
	ctx context.Context,
	item domain.Project,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saved = append(s.Saved, item)
	return nil
}

func (s *ProjectStore) Delete(
	ctx context.Context,
	id string,
) error {
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
	return s.Saved, nil
}

// RepositoryStore is a fake store.Store[domain.Repository, string].
type RepositoryStore struct {
	Saved   []domain.Repository
	SaveErr error
}

// NewRepositoryStore returns an empty RepositoryStore.
func NewRepositoryStore() *RepositoryStore {
	return &RepositoryStore{}
}

func (s *RepositoryStore) Save(
	ctx context.Context,
	item domain.Repository,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
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
	return s.Saved, nil
}

// WorkspaceRepo is a fake of the subset of workspace.Workspace used on import.
type WorkspaceRepo struct {
	Created   []domain.Workspace
	CreateErr error
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
	Detached              []string          // worktree paths detached to HEAD
	CheckedOut            []WorktreeAddCall // (path, branch) re-attach calls
	WorktreeAdds          []WorktreeAddCall // (path, branch) worktrees materialised
	WorktreeRemoves       []string          // worktree paths force-removed
	FetchedRefs           []string          // branches fetched from origin (FetchRef)
	FastForwardedBranches []string          // branches fast-forwarded from origin (FastForwardBranch)
	RemoteBranches        map[string]bool   // branch -> exists on origin (default false)
	RevParseShas          map[string]string // rev -> sha (default "")
	DetachErr             error             // forces DetachWorktree to fail
	// WorktreeAddErrByBranch forces WorktreeAdd to fail for specific branches.
	WorktreeAddErrByBranch map[string]error
	// Pruned records repo paths WorktreePrune was called on.
	Pruned []string
	// DeadRegistrations maps a dead worktree dir -> branch it "holds"; WorktreePrune
	// removes them and merges the survivors into the list holder.Resolve sees.
	DeadRegistrations map[string]string
	// FastForwardErr forces FastForwardBranch to fail (best-effort FF path).
	FastForwardErr error
}

// WorktreeAddCall records a fake WorktreeAdd invocation.
type WorktreeAddCall struct {
	Path   string
	Branch string
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

func (g *GitEngine) FetchRef(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
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

func (g *GitEngine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
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
	Resolved   bool
	ResolvedID string

	GetFn     func(ctx context.Context, id string) (domain.Workspace, error)
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

func (s *WorkspaceSyncer) SyncWorkingTreeState(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	s.Synced = true
	s.SyncedID = id
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
// usecase.
type TerminalEngine struct {
	CreateFn          func(ctx context.Context, wsID, dir string, prof *domain.TerminalProfile) (string, error)
	KillFn            func(ctx context.Context, sessionID string) error
	LoadPlaceholderFn func(ctx context.Context, m engineterminal.SessionMeta, scrollback []byte) error
}

// NewTerminalEngine returns an empty TerminalEngine.
func NewTerminalEngine() *TerminalEngine {
	return &TerminalEngine{}
}

func (e *TerminalEngine) Create(
	ctx context.Context,
	workspaceID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	return e.CreateFn(ctx, workspaceID, workspaceDir, prof)
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
