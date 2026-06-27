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
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
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
	// FindByKey loads a project by id so a standalone repo import (ImportRepo)
	// can resolve the owning project before running the per-repo import.
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Project, error)
}

// RepositoryStore is the repository persistence surface the import usecase needs.
type RepositoryStore interface {
	Save(
		ctx context.Context,
		item domain.Repository,
	) error
	// Delete removes a repository row by id. Used to roll back a repo whose
	// worktree adoption failed, so a failed import never persists an orphaned
	// repository (one with no workspaces, hence unnavigable).
	Delete(
		ctx context.Context,
		id string,
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

// AvatarBytesFetcher downloads the repo owner's avatar image bytes plus the
// response content-type. It is best-effort: on absence (no origin/no gh) it
// returns (nil, "", nil) so the import falls back to a generated avatar.
type AvatarBytesFetcher func(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error)

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
	// CrowbarHome resolves the ~/.crowbar root used to derive the entity-scoped
	// repo icon path. Defaults to worktreepath.DefaultCrowbarHome when nil.
	CrowbarHome func() (string, error)
	// FetchAvatarBytes downloads the GitHub owner avatar bytes for a repo with
	// no local icon. Defaults to avatar.FetchOwnerAvatarBytes when nil.
	FetchAvatarBytes AvatarBytesFetcher
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
	// ImportRepo runs the full per-repo import for a single repo path under an
	// already-persisted project: it persists the Repository, writes the repo
	// icon (local icon or best-effort GitHub owner avatar), adopts each existing
	// worktree as a Workspace (so the default/checked-out branch becomes a
	// workspace), and imports protected-branch stubs. It returns the created
	// Repository so the caller can broadcast its DTO (§14 Step 3).
	ImportRepo(
		ctx context.Context,
		projectID string,
		repoPath string,
	) (domain.Repository, error)
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
	if deps.CrowbarHome == nil {
		deps.CrowbarHome = worktreepath.DefaultCrowbarHome
	}
	if deps.FetchAvatarBytes == nil {
		deps.FetchAvatarBytes = avatar.FetchOwnerAvatarBytes
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
	if err := u.createHomeWorkspace(ctx, project); err != nil {
		return domain.Project{}, err
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
	if err := u.createHomeWorkspace(ctx, project); err != nil {
		return domain.Project{}, err
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
		if _, err := u.importOneRepo(ctx, project, repoPath); err != nil {
			slog.WarnContext(
				ctx, "project import: skipping repo after partial failure",
				"repo_path", repoPath,
				"error", err,
			)
		}
	}
	return nil
}

// ImportRepo loads the owning project by id then runs the shared per-repo import
// for a single repo path, returning the created Repository (00 §14 Step 3).
func (u *projectImport) ImportRepo(
	ctx context.Context,
	projectID string,
	repoPath string,
) (domain.Repository, error) {
	project, err := u.deps.Projects.FindByKey(ctx, projectID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("import repo: load project: %w", err)
	}
	if project == nil {
		return domain.Repository{}, fmt.Errorf("import repo: project %q not found", projectID)
	}
	return u.importOneRepo(ctx, *project, repoPath)
}

func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	repoPath string,
) (domain.Repository, error) {
	name := filepath.Base(repoPath)
	repoID := uuid.NewString()
	runner := u.deps.RefRunner(repoPath)
	repo := domain.Repository{
		ID:            repoID,
		ProjectID:     project.ID,
		Name:          name,
		Path:          repoPath,
		DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarHasIcon: u.writeRepoIcon(ctx, project.ID, repoID, repoPath),
		RemoteURL:     gitRemoteURL(repoPath),
	}
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return domain.Repository{}, fmt.Errorf("project import: save repository: %w", err)
	}
	// Roll back the repo row if WORKTREE ADOPTION fails. A repository with no
	// workspaces is unnavigable (workspaces are the UI's unit) and unusable; never
	// leave one persisted. Without this, a failed adoption (git error, ws-create
	// failure) orphans the repo — visible-but-broken in the multi-repo Import, and
	// accumulating one stale row per retry in the single-repo ImportRepo path. Once
	// adoption succeeds the repo HAS workspaces and must be kept, so committed is
	// set before the best-effort protected-branch stubs (whose failure must NOT
	// roll back a repo that already has valid workspaces).
	committed := false
	defer func() {
		if !committed {
			_ = u.deps.Repos.Delete(ctx, repo.ID)
		}
	}()
	adopted, err := u.adoptWorktrees(ctx, repo)
	if err != nil {
		return domain.Repository{}, err
	}
	if len(adopted) == 0 {
		// No worktree could be adopted (all per-worktree adoptions failed, or the
		// repo has no adoptable worktree). A repo with zero workspaces is unnavigable;
		// roll it back rather than persist a broken row.
		return domain.Repository{}, fmt.Errorf("project import: repo %q: no workspaces adopted", repo.Name)
	}
	committed = true
	if err := u.importProtectedBranchStubs(ctx, repo, adopted); err != nil {
		return domain.Repository{}, err
	}
	return repo, nil
}

// writeRepoIcon resolves the repo icon bytes (local repo icon first, then a
// best-effort GitHub owner-avatar download) and writes them to the
// entity-scoped icon path <home>/projects/<P>/<R>/icon. It returns true when an
// icon was written. All failures degrade to false (generated avatar) and never
// fail the import.
func (u *projectImport) writeRepoIcon(
	ctx context.Context,
	projectID string,
	repoID string,
	repoPath string,
) bool {
	data := u.resolveIconBytes(ctx, repoPath)
	if len(data) == 0 {
		return false
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return false
	}
	iconPath := worktreepath.RepoIconPath(home, projectID, repoID)
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil {
		slog.WarnContext(ctx, "project import: create repo icon dir failed", "error", err)
		return false
	}
	if err := os.WriteFile(iconPath, data, 0o644); err != nil {
		slog.WarnContext(ctx, "project import: write repo icon failed", "error", err)
		return false
	}
	return true
}

// resolveIconBytes returns the icon bytes for a repo: the local repo icon if
// one is on disk, otherwise the best-effort GitHub owner avatar. Returns nil
// when neither is available.
func (u *projectImport) resolveIconBytes(
	ctx context.Context,
	repoPath string,
) []byte {
	if src := avatar.ScanRepoIcon(repoPath); src != "" {
		if data, err := os.ReadFile(src); err == nil {
			return data
		}
	}
	data, _, err := u.deps.FetchAvatarBytes(ctx, repoPath)
	if err != nil {
		return nil
	}
	return data
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
		// Auto-import only the repo's main worktree (always — it is the default
		// workspace / repo header) and worktrees whose branch is protected on the
		// remote. Other local worktrees (feature branches, agent checkouts) are
		// left for the user to add explicitly, rather than flooding the sidebar
		// with every checkout on disk at import time.
		if !samePath(wt.Path, repo.Path) && !locked[wt.Branch] {
			continue
		}
		if err := u.adoptOneWorktree(ctx, repo, wt, locked); err != nil {
			// Per-worktree adoption is best-effort: skip the worktree that failed
			// rather than aborting the whole repo. Aborting after an earlier worktree
			// already created a workspace would, with the caller's rollback, delete
			// the repo and ORPHAN that workspace. Skipping keeps every successfully
			// adopted worktree; if NONE succeed the caller rolls the repo back.
			slog.WarnContext(ctx, "project import: skipping worktree after adopt failure",
				"repo", repo.Name, "branch", wt.Branch, "err", err)
			continue
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

// samePath reports whether two filesystem paths refer to the same location,
// resolving symlinks first so a repo imported under a symlinked root still
// matches the path git reports for its main worktree. This matters because git
// worktree list emits the fully-resolved path (e.g. macOS /var -> /private/var,
// or a symlinked home / network mount), while repo.Path is the path as imported;
// a naive string compare would then never flag the main worktree as default.
// Falls back to a lexical clean when a path cannot be resolved (e.g. it no
// longer exists on disk).
func samePath(a string, b string) bool {
	return resolvePath(a) == resolvePath(b)
}

func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
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
		IsDefault:    samePath(wt.Path, repo.Path),
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

// createHomeWorkspace persists the project-level home workspace rooted at the
// project's own path. It has no repo, branch, or git operations.
func (u *projectImport) createHomeWorkspace(ctx context.Context, project domain.Project) error {
	_, err := u.deps.Workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		ProjectID:    project.ID,
		WorktreePath: project.Path,
		Kind:         domain.WorkspaceKindHome,
	}, u.deps.Now())
	if err != nil {
		return fmt.Errorf("project create home workspace: %w", err)
	}
	return nil
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
