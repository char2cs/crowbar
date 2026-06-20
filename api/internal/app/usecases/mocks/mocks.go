// Package mocks provides hand-written fakes for the usecase collaborators
// (GORM stores, workspace repository, git and provider engines) used in tests.
package mocks

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
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
	SetParentFromPRFn func(ctx context.Context, id string, parentID string) (domain.Workspace, error)
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

func (r *ProviderSyncWorkspaceRepo) SetParentFromPR(ctx context.Context, id string, parentID string) (domain.Workspace, error) {
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

// ChatForkArgs records the arguments passed to a Fork call.
type ChatForkArgs struct {
	ID       string
	WsID     string
	ParentID string
	Title    string
}

// ChatRepo is a fake of the chat repo surface used by the chat usecase.
type ChatRepo struct {
	Created []domain.Chat
	Forked  []ChatForkArgs
	Deleted []string

	CreateErr error
	ForkErr   error
	RenameErr error
	DeleteErr error

	// RenameWsID is the WsID carried by the aggregate returned from Rename,
	// mirroring the real repo which returns the full chat (with its workspace).
	RenameWsID string

	GetFn func(ctx context.Context, id string) (domain.Chat, error)

	ListByWorkspaceFn func(ctx context.Context, wsID string) ([]domain.Chat, error)
}

// NewChatRepo returns an empty ChatRepo.
func NewChatRepo() *ChatRepo {
	return &ChatRepo{}
}

func (r *ChatRepo) Create(
	ctx context.Context,
	id string,
	wsID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	if r.CreateErr != nil {
		return domain.Chat{}, r.CreateErr
	}
	c := domain.Chat{ID: id, WsID: wsID, Title: title, CreatedAt: now}
	r.Created = append(r.Created, c)
	return c, nil
}

func (r *ChatRepo) Fork(
	ctx context.Context,
	id string,
	wsID string,
	parentID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	if r.ForkErr != nil {
		return domain.Chat{}, r.ForkErr
	}
	r.Forked = append(r.Forked, ChatForkArgs{ID: id, WsID: wsID, ParentID: parentID, Title: title})
	return domain.Chat{ID: id, WsID: wsID, ParentID: parentID, Title: title, CreatedAt: now}, nil
}

func (r *ChatRepo) Rename(
	ctx context.Context,
	id string,
	title string,
) (domain.Chat, error) {
	if r.RenameErr != nil {
		return domain.Chat{}, r.RenameErr
	}
	return domain.Chat{ID: id, WsID: r.RenameWsID, Title: title}, nil
}

func (r *ChatRepo) Delete(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Chat, error) {
	if r.DeleteErr != nil {
		return domain.Chat{}, r.DeleteErr
	}
	r.Deleted = append(r.Deleted, id)
	return domain.Chat{ID: id}, nil
}

func (r *ChatRepo) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return r.GetFn(ctx, id)
}

func (r *ChatRepo) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	return r.ListByWorkspaceFn(ctx, wsID)
}

// ChatWorkspaceRepo is a fake of the workspace surface used by the chat usecase.
type ChatWorkspaceRepo struct {
	TouchedID string
	GetFn     func(ctx context.Context, id string) (domain.Workspace, error)
}

// NewChatWorkspaceRepo returns an empty ChatWorkspaceRepo.
func NewChatWorkspaceRepo() *ChatWorkspaceRepo {
	return &ChatWorkspaceRepo{}
}

func (r *ChatWorkspaceRepo) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	if r.GetFn == nil {
		return domain.Workspace{}, errors.New("mocks: ChatWorkspaceRepo.GetFn not set")
	}
	return r.GetFn(ctx, id)
}

func (r *ChatWorkspaceRepo) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	r.TouchedID = id
	return domain.Workspace{ID: id}, nil
}

// WorkspaceSyncer is a fake of the workspace syncer surface used by the file and
// git usecases.
type WorkspaceSyncer struct {
	Synced   bool
	SyncedID string

	GetFn  func(ctx context.Context, id string) (domain.Workspace, error)
	SyncFn func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
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

// FsEngine is a fake of the filesystem-engine surface used by the file usecase.
type FsEngine struct {
	TreeFn func(repoPath, dirPath string, provider file.FileStatusProvider) ([]domain.FileNode, error)

	ReadContentFn  func(repoPath, filePath string) (domain.FileContent, error)
	WriteContentFn func(repoPath, filePath, content string) error
	CreateFileFn   func(repoPath, filePath string) error
	CreateDirFn    func(repoPath, dirPath string) error
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
) error {
	return e.WriteContentFn(repoPath, filePath, content)
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
	CreateFn func(ctx context.Context, wsID, dir string, prof *domain.TerminalProfile) (string, error)
	KillFn   func(ctx context.Context, sessionID string) error
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
