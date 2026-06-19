package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// Store is the project persistence surface the import usecase needs.
type Store interface {
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
	// OwnerAvatarURL returns the repo owner's avatar URL, or "" on failure.
	OwnerAvatarURL(
		ctx context.Context,
		repoPath string,
	) (string, error)
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

// ImportDeps wires the import usecase's collaborators.
type ImportDeps struct {
	Projects   Store
	Repos      RepositoryStore
	Workspaces WorkspaceCreator
	Git        ImportGitEngine
	Provider   ImportProviderEngine
	Discover   DiscoverFunc
	RefRunner  RefRunnerFactory
	Now        func() time.Time
	// Stat probes the import path before anything is persisted, so a failed
	// import leaves no project behind. Defaults to os.Stat when nil; tests
	// stub it to avoid touching the real filesystem.
	Stat func(name string) (os.FileInfo, error)
}

// ImportUsecase imports a directory tree as a Project: it creates the
// Project row, discovers repos, persists a Repository per repo, and adopts each
// existing git worktree as a Workspace row (00 §5.7).
type ImportUsecase interface {
	Import(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
	// Create is the lightweight variant of Import: it validates the path and
	// persists only the Project row. Repo discovery and workspace stub creation
	// are skipped — the project-level welcome screen handles them later.
	Create(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
}

type projectImport struct {
	deps ImportDeps
}

// NewImport builds an ImportUsecase from its dependencies.
func NewImport(
	deps ImportDeps,
) ImportUsecase {
	if deps.Stat == nil {
		deps.Stat = os.Stat
	}
	return &projectImport{deps: deps}
}

func (u *projectImport) Create(
	ctx context.Context,
	name string,
	path string,
) (domain.Project, error) {
	if err := u.validateImportPath(path); err != nil {
		return domain.Project{}, err
	}
	project := domain.Project{
		ID:           uuid.NewString(),
		Name:         name,
		Path:         path,
		LastActivity: u.deps.Now(),
	}
	if err := u.deps.Projects.Save(ctx, project); err != nil {
		return domain.Project{}, fmt.Errorf("project create: save project: %w", err)
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
			slog.WarnContext(
				ctx, "project import: skipping repo after partial failure",
				"repo_path", repoPath,
				"error", err,
			)
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
	avatarURL := avatar.ScanRepoIcon(repoPath)
	if avatarURL == "" {
		avatarURL, _ = u.deps.Provider.OwnerAvatarURL(ctx, repoPath)
	}
	repo := domain.Repository{
		ID:            uuid.NewString(),
		ProjectID:     project.ID,
		Name:          name,
		Path:          repoPath,
		DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarURL:     avatarURL,
		RemoteURL:     gitRemoteURL(repoPath),
	}
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return fmt.Errorf("project import: save repository: %w", err)
	}
	adopted, err := u.adoptWorktrees(ctx, repo)
	if err != nil {
		return err
	}
	return u.importProtectedBranchStubs(ctx, repo, adopted)
}

func (u *projectImport) adoptWorktrees(
	ctx context.Context,
	repo domain.Repository,
) (map[string]bool, error) {
	worktrees, err := u.deps.Git.WorktreeList(ctx, repo.Path)
	if err != nil {
		return nil, fmt.Errorf("project import: list worktrees: %w", err)
	}
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		return nil, fmt.Errorf("project import: protected branches: %w", err)
	}
	locked := toSet(protected)
	adopted := make(map[string]bool)
	for _, wt := range worktrees {
		if err := u.adoptOneWorktree(ctx, repo, wt, locked); err != nil {
			return nil, err
		}
		if wt.Branch != "" && !wt.Prunable {
			adopted[wt.Branch] = true
		}
	}
	return adopted, nil
}

// importProtectedBranchStubs creates locked workspace records for protected
// branches that do not already have a local worktree.
func (u *projectImport) importProtectedBranchStubs(
	ctx context.Context,
	repo domain.Repository,
	adopted map[string]bool,
) error {
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		return nil // soft: don't fail the entire import
	}
	for _, branch := range protected {
		if adopted[branch] {
			continue
		}
		in := workspace.CreateInput{
			ID:           uuid.NewString(),
			RepoID:       repo.ID,
			ProjectID:    repo.ProjectID,
			Branch:       branch,
			WorktreePath: repo.Path,
			Protected:    true,
		}
		if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
			slog.WarnContext(ctx, "project import: skip protected branch stub",
				"branch", branch, "error", err)
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
	// A prunable worktree points at a checkout that no longer exists on disk
	// (e.g. a deleted temp dir). Adopting it would create a workspace whose
	// file tree and git status fail every read, so skip it.
	if wt.Prunable {
		return nil
	}
	in := workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       wt.Branch,
		WorktreePath: wt.Path,
		ForkPointSha: u.forkPoint(ctx, repo, wt.Branch),
		Protected:    locked[wt.Branch],
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

func toSet(
	values []string,
) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// gitRemoteURL returns the origin remote URL for the repo at path, or ""
// on any failure so callers can fall back gracefully.
func gitRemoteURL(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
