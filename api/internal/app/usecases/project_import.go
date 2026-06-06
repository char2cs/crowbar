package usecases

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// importMaxDepth bounds repo discovery to three directory levels below the
// import root — deep enough for the org/group/repo layouts crowbar targets,
// shallow enough to avoid descending into vendored or nested trees (00 §5.7).
const importMaxDepth = 3

// defaultBranchCandidates is the ordered fallback list when origin/HEAD and the
// repo HEAD cannot resolve a default branch (00 §5.2).
var defaultBranchCandidates = []string{"main", "develop", "master"}

// ProjectStore is the project persistence surface the import usecase needs.
type ProjectStore interface {
	Save(
		ctx context.Context,
		item domain.Project,
	) error
}

// RepositoryStore is the repository persistence surface the import usecase needs.
type RepositoryStore interface {
	Save(
		ctx context.Context,
		item domain.Repository,
	) error
}

// WorkspaceCreator is the workspace-creation surface the import usecase needs.
type WorkspaceCreator interface {
	Create(
		ctx context.Context,
		in workspace.CreateInput,
		now time.Time,
	) (domain.Workspace, error)
}

// ImportGitEngine is the git surface the import usecase consumes.
type ImportGitEngine interface {
	WorktreeList(
		ctx context.Context,
		repoPath string,
	) ([]gitengine.WorktreeEntry, error)
	MergeBase(
		ctx context.Context,
		repoPath string,
		a string,
		b string,
	) (string, error)
}

// ImportProviderEngine is the provider surface the import usecase consumes.
type ImportProviderEngine interface {
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
}

// DiscoverFunc walks root and returns the repo roots found within maxDepth.
type DiscoverFunc func(
	root string,
	maxDepth int,
) ([]string, error)

// RefRunnerFactory builds a defaultbranch.RefRunner bound to a repo path.
type RefRunnerFactory func(
	repoPath string,
) defaultbranch.RefRunner

// ProjectImportDeps wires the import usecase's collaborators.
type ProjectImportDeps struct {
	Projects   ProjectStore
	Repos      RepositoryStore
	Workspaces WorkspaceCreator
	Git        ImportGitEngine
	Provider   ImportProviderEngine
	Discover   DiscoverFunc
	RefRunner  RefRunnerFactory
	Now        func() time.Time
}

// ProjectImportUsecase imports a directory tree as a Project: it creates the
// Project row, discovers repos, persists a Repository per repo, and adopts each
// existing git worktree as a Workspace row (00 §5.7).
type ProjectImportUsecase interface {
	Import(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
}

type projectImport struct {
	deps ProjectImportDeps
}

// NewProjectImport builds a ProjectImportUsecase from its dependencies.
func NewProjectImport(
	deps ProjectImportDeps,
) ProjectImportUsecase {
	return &projectImport{deps: deps}
}

func (u *projectImport) Import(
	ctx context.Context,
	name string,
	path string,
) (domain.Project, error) {
	project := domain.Project{
		ID:           uuid.NewString(),
		Name:         name,
		Path:         path,
		LastActivity: u.deps.Now(),
	}
	if err := u.deps.Projects.Save(ctx, project); err != nil {
		return domain.Project{}, fmt.Errorf("project import: save project: %w", err)
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
		if err := u.importOneRepo(ctx, project, repoPath); err != nil {
			return err
		}
	}
	return nil
}

func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	repoPath string,
) error {
	name := filepath.Base(repoPath)
	runner := u.deps.RefRunner(repoPath)
	repo := domain.Repository{
		ID:            uuid.NewString(),
		ProjectID:     project.ID,
		Name:          name,
		Path:          repoPath,
		DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
	}
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return fmt.Errorf("project import: save repository: %w", err)
	}
	return u.adoptWorktrees(ctx, repo)
}

func (u *projectImport) adoptWorktrees(
	ctx context.Context,
	repo domain.Repository,
) error {
	worktrees, err := u.deps.Git.WorktreeList(ctx, repo.Path)
	if err != nil {
		return fmt.Errorf("project import: list worktrees: %w", err)
	}
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		return fmt.Errorf("project import: protected branches: %w", err)
	}
	locked := toSet(protected)
	for _, wt := range worktrees {
		if err := u.adoptOneWorktree(ctx, repo, wt, locked); err != nil {
			return err
		}
	}
	return nil
}

func (u *projectImport) adoptOneWorktree(
	ctx context.Context,
	repo domain.Repository,
	wt gitengine.WorktreeEntry,
	locked map[string]bool,
) error {
	if wt.Branch == "" {
		return nil
	}
	in := workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       wt.Branch,
		WorktreePath: wt.Path,
		ForkPointSha: u.forkPoint(ctx, repo, wt.Branch),
		Locked:       locked[wt.Branch],
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("project import: adopt worktree: %w", err)
	}
	return nil
}

func (u *projectImport) forkPoint(
	ctx context.Context,
	repo domain.Repository,
	branch string,
) string {
	if branch == repo.DefaultBranch {
		return ""
	}
	sha, err := u.deps.Git.MergeBase(ctx, repo.Path, branch, repo.DefaultBranch)
	if err != nil {
		return ""
	}
	return sha
}

func toSet(
	values []string,
) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}
