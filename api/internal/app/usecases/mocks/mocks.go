// Package mocks provides hand-written fakes for the usecase collaborators
// (GORM stores, workspace repository, git and provider engines) used in tests.
package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
	provider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// ProjectStore is a fake store.Store[domain.Project, string].
type ProjectStore struct {
	Saved   []domain.Project
	SaveErr error
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
	if r.CreateErr != nil {
		return domain.Workspace{}, r.CreateErr
	}
	ws := domain.Workspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Locked:        in.Locked,
		MergeStrategy: in.MergeStrategy,
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
}

// NewGitEngine returns an empty GitEngine.
func NewGitEngine() *GitEngine {
	return &GitEngine{}
}

func (g *GitEngine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]gitengine.WorktreeEntry, error) {
	if g.WorktreeListErr != nil {
		return nil, g.WorktreeListErr
	}
	return g.Worktrees, nil
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

// ProviderEngine is a fake of the provider operations the import usecase consumes.
type ProviderEngine struct {
	Protected    []string
	ProtectedErr error
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

// ProviderSyncWorkspaceRepo is a fake of the workspace.Workspace surface used
// by ProviderSyncUsecase.
type ProviderSyncWorkspaceRepo struct {
	GetFn          func(ctx context.Context, id string) (domain.Workspace, error)
	SyncProviderFn func(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
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
	ListFn func(ctx context.Context) ([]domain.Workspace, error)
	GetFn  func(ctx context.Context, id string) (domain.Workspace, error)

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

// WorkingTreeGitEngine is a fake of the git WorkingTreeSummary surface.
type WorkingTreeGitEngine struct {
	WorkingTreeSummaryFn func(
		ctx context.Context,
		repoPath string,
		forkPointSha string,
	) (int, int, bool, bool, error)
}

// NewWorkingTreeGitEngine returns an empty WorkingTreeGitEngine.
func NewWorkingTreeGitEngine() *WorkingTreeGitEngine {
	return &WorkingTreeGitEngine{}
}

func (g *WorkingTreeGitEngine) WorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	return g.WorkingTreeSummaryFn(ctx, repoPath, forkPointSha)
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
