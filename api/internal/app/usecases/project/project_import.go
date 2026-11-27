package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

const importMaxDepth = 3

var defaultBranchCandidates = []string{"main", "develop", "master"}

type Store interface {
	Save(
		ctx context.Context,
		item domain.Project,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Project, error)
}

type RepositoryStore interface {
	Save(
		ctx context.Context,
		item domain.Repository,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindAll(
		ctx context.Context,
	) ([]domain.Repository, error)
}

type WorkspaceCreator interface {
	Create(
		ctx context.Context,
		in workspace.CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

type ImportGitEngine interface {
	WorktreeList(
		ctx context.Context,
		repoPath string,
	) ([]gitengine.WorktreeEntry, error)
	DetachWorktree(
		ctx context.Context,
		worktreePath string,
	) error
	CheckoutBranch(
		ctx context.Context,
		worktreePath string,
		branch string,
	) error
	RemoteBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	RemoteTrackingBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
	LocalBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
	WorktreeAddAtRef(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
		startRef string,
	) (string, error)
	SetUpstream(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	WorktreeAdd(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
	) error
	WorktreePrune(
		ctx context.Context,
		repoPath string,
	) error
	WorktreeRemove(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		force bool,
	) error
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
	MergeBase(
		ctx context.Context,
		repoPath string,
		a string,
		b string,
	) (string, error)
}

type ImportProviderEngine interface {
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
}

type AvatarBytesFetcher func(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error)

type DiscoverFunc func(
	root string,
	maxDepth int,
) ([]string, error)

type RefRunnerFactory func(
	repoPath string,
) defaultbranch.RefRunner

type HomeSlots interface {
	NextHomeSlot(
		ctx context.Context,
		projectID string,
	) (int, error)
}

type ImportDeps struct {
	Projects         Store
	Repos            RepositoryStore
	Workspaces       WorkspaceCreator
	Git              ImportGitEngine
	Provider         ImportProviderEngine
	Discover         DiscoverFunc
	RefRunner        RefRunnerFactory
	Nodes            NodePlacements
	HomeSlots        HomeSlots
	Now              func() time.Time
	Stat             func(name string) (os.FileInfo, error)
	CrowbarHome      func() (string, error)
	FetchAvatarBytes AvatarBytesFetcher
}

type ImportUsecase interface {
	Import(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
	Create(
		ctx context.Context,
		id string,
		name string,
		path string,
	) (domain.Project, error)
	ImportRepo(
		ctx context.Context,
		in RepoImport,
		progress RepoProgress,
	) (domain.Repository, error)
	CheckRepoImportable(
		ctx context.Context,
		projectID string,
		repoPath string,
	) error
	CreateHome(
		ctx context.Context,
		project domain.Project,
	) (domain.Workspace, error)
	SetOwningChats(chats OwningChats)
}

type projectImport struct {
	deps        ImportDeps
	owningChats OwningChats
}

func (u *projectImport) SetOwningChats(chats OwningChats) {
	u.owningChats = chats
}

func NewImport(
	deps ImportDeps,
) ImportUsecase {
	if deps.Stat == nil {
		deps.Stat = os.Stat
	}
	if deps.CrowbarHome == nil {
		panic("project: ImportDeps.CrowbarHome is required")
	}
	if deps.FetchAvatarBytes == nil {
		deps.FetchAvatarBytes = avatar.FetchOwnerAvatarBytes
	}
	return &projectImport{deps: deps}
}

func (u *projectImport) Create(
	ctx context.Context,
	id string,
	name string,
	path string,
) (domain.Project, error) {
	if err := u.validateImportPath(path); err != nil {
		return domain.Project{}, err
	}
	project, err := u.saveProjectWithHome(ctx, id, name, path)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project create: %w", err)
	}
	return project, nil
}

func (u *projectImport) Import(
	ctx context.Context,
	name string,
	path string,
) (domain.Project, error) {
	if err := u.validateImportPath(path); err != nil {
		return domain.Project{}, err
	}
	project, err := u.saveProjectWithHome(ctx, "", name, path)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project import: %w", err)
	}
	if err := u.importRepos(ctx, project, path); err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func (u *projectImport) importRepos(
	ctx context.Context,
	project domain.Project,
	path string,
) error {
	repoPaths, err := u.deps.Discover(path, importMaxDepth)
	if err != nil {
		return fmt.Errorf("project import: discover repos: %w", err)
	}
	for _, repoPath := range repoPaths {
		if _, err := u.importOneRepo(ctx, project, RepoImport{Path: repoPath}, loggedProgress{ctx}); err != nil {
			slog.WarnContext(
				ctx, "project import: skipping repo after partial failure",
				"repo_path", repoPath,
				"error", err,
			)
		}
	}
	return nil
}

func (u *projectImport) ImportRepo(
	ctx context.Context,
	in RepoImport,
	progress RepoProgress,
) (domain.Repository, error) {
	project, err := u.deps.Projects.FindByKey(ctx, in.ProjectID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("import repo: load project: %w", err)
	}
	if project == nil {
		return domain.Repository{}, fmt.Errorf("import repo: project %q not found", in.ProjectID)
	}
	return u.importOneRepo(ctx, *project, in, progress)
}

type loggedProgress struct {
	ctx context.Context
}

func (l loggedProgress) Workspace(id, branch string, err error) {
	if err != nil {
		slog.WarnContext(l.ctx, "project import: protected branch has no workspace", "workspace", id, "branch", branch, "error", err)
	}
}

func (u *projectImport) saveProjectWithHome(
	ctx context.Context,
	id string,
	name string,
	path string,
) (domain.Project, error) {
	if id == "" {
		id = uuid.NewString()
	}
	project := domain.Project{
		ID:           id,
		Name:         name,
		Path:         path,
		LastActivity: u.deps.Now(),
	}
	home, err := u.CreateHome(ctx, project)
	if err != nil {
		return domain.Project{}, err
	}
	if err := u.deps.Projects.Save(ctx, project); err != nil {
		if dErr := u.deps.Workspaces.Delete(ctx, home.ID); dErr != nil {
			slog.ErrorContext(ctx, "project import: roll back the home workspace",
				"workspace_id", home.ID, "err", dErr)
		}
		return domain.Project{}, fmt.Errorf("save project: %w", err)
	}
	return project, nil
}

func (u *projectImport) CreateHome(
	ctx context.Context,
	project domain.Project,
) (domain.Workspace, error) {
	home, err := u.createOwnedWorkspace(ctx, workspace.CreateInput{
		ID:           workspace.ProjectHomeID(project.ID),
		ProjectID:    project.ID,
		WorktreePath: project.Path,
		Kind:         domain.WorkspaceKindHome,
		Provisioning: domain.WorkspaceShared,
	}, u.deps.Now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("home workspace: %w", err)
	}
	return home, nil
}

func (u *projectImport) validateImportPath(
	path string,
) error {
	_, err := u.deps.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrFolderNotFound
	}
	if err != nil {
		return fmt.Errorf("project import: stat path: %w", err)
	}
	return nil
}
