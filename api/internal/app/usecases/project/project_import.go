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

// importMaxDepth bounds repo discovery to three directory levels below the
// import root — deep enough for the org/group/repo layouts crowbar targets,
// shallow enough to avoid descending into vendored or nested trees.
const importMaxDepth = 3

// defaultBranchCandidates is the ordered fallback list when origin/HEAD and the
// repo HEAD cannot resolve a default branch.
var defaultBranchCandidates = []string{"main", "develop", "master"}

// Store is the project persistence surface the import usecase needs.
type Store interface {
	Save(
		ctx context.Context,
		item domain.Project,
	) error
	// Delete removes a project row by id. Used to roll back a project whose
	// home workspace creation failed, so a failed create/import never persists
	// an orphaned project row.
	Delete(
		ctx context.Context,
		id string,
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
	// FindAll lists every repository, used to dedup an import: re-adding a folder
	// already imported under the project must be a no-op, not a duplicate row.
	FindAll(
		ctx context.Context,
	) ([]domain.Repository, error)
}

// WorkspaceCreator is the workspace-creation surface the import usecase needs.
type WorkspaceCreator interface {
	Create(
		ctx context.Context,
		in workspace.CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	// Delete tombstones a workspace row, solely to roll back one that could not be
	// attached to its owning chat (discardUnownedWorkspace), which would otherwise orphan it.
	Delete(
		ctx context.Context,
		id string,
	) error
}

// ImportGitEngine is the git surface the import usecase consumes: the full worktree-
// mutation surface, since import detaches the repo home off a protected branch and
// provisions one managed worktree per protected branch.
type ImportGitEngine interface {
	WorktreeList(
		ctx context.Context,
		repoPath string,
	) ([]gitengine.WorktreeEntry, error)
	// DetachWorktree puts the worktree at worktreePath on a detached HEAD,
	// releasing the branch it held so a managed worktree can claim it.
	DetachWorktree(
		ctx context.Context,
		worktreePath string,
	) error
	// CheckoutBranch re-attaches the worktree at worktreePath onto branch. Used to
	// restore the repo home if it was detached but the home row then failed to
	// persist — the user's real repo must never be left on a detached HEAD.
	CheckoutBranch(
		ctx context.Context,
		worktreePath string,
		branch string,
	) error
	// RemoteBranchExists reports whether branch exists on the origin remote —
	// decides fetch-then-checkout vs checkout-local before adding a worktree.
	RemoteBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
	// FetchRef refreshes refs/remotes/origin/<branch>. Unlike a fetch that also
	// moves the local branch ref, git never refuses it — not even when the
	// branch is checked out somewhere.
	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	// RemoteTrackingBranchExists reports whether origin/<branch> resolves
	// locally — "is there remote content to check out?".
	RemoteTrackingBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
	// LocalBranchExists reports whether refs/heads/<branch> is present.
	LocalBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
	// WorktreeAddAtRef checks branch out into a new worktree with the branch ref
	// RESET to startRef, so the worktree holds the REMOTE branch even when a
	// diverged local branch of the same name exists.
	WorktreeAddAtRef(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
		startRef string,
	) (string, error)
	// SetUpstream links branch to origin/<branch>. Required after
	// WorktreeAddAtRef, which starts from a SHA and so sets no tracking info of
	// its own — without it `git pull` in the worktree has nothing to merge with.
	SetUpstream(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	// WorktreeAdd checks an existing branch out into a new worktree at worktreePath.
	WorktreeAdd(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
	) error
	// WorktreePrune reaps registrations whose directory is gone, so a branch held only
	// by a deleted worktree dir (e.g. after rm -rf ~/.crowbar) is freed before provisioning.
	WorktreePrune(
		ctx context.Context,
		repoPath string,
	) error
	// WorktreeRemove force-removes a worktree, used to undo a managed worktree
	// when its workspace row fails to persist.
	WorktreeRemove(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		force bool,
	) error
	// RevParse resolves a rev (e.g. a branch name) to a commit SHA, recorded as a
	// managed worktree's fork point.
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
	// MergeBase reports whether a -B checkout left a local tip behind.
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

// HomeSlots is the one read the import needs off the project usecase: the
// next free index at a project's home root.
type HomeSlots interface {
	NextHomeSlot(
		ctx context.Context,
		projectID string,
	) (int, error)
}

// ImportDeps wires the import usecase's collaborators.
type ImportDeps struct {
	Projects   Store
	Repos      RepositoryStore
	Workspaces WorkspaceCreator
	Git        ImportGitEngine
	Provider   ImportProviderEngine
	Discover   DiscoverFunc
	RefRunner  RefRunnerFactory
	// Nodes mints the Node{Kind: NodeKindRepo} row that owns a newly imported
	// repo's own sidebar position, at the same point its Repository row is
	// persisted — see NodePlacements.
	Nodes NodePlacements
	// HomeSlots answers where a new repo's Node is minted at its project's
	// home root (see Usecase.NextHomeSlot). nil mints at 0.
	HomeSlots HomeSlots
	Now       func() time.Time
	// Stat probes the import path before anything is persisted, so a failed
	// import leaves no project behind. Defaults to os.Stat when nil; tests
	// stub it to avoid touching the real filesystem.
	Stat func(name string) (os.FileInfo, error)
	// CrowbarHome resolves the Crowbar home used to derive the entity-scoped
	// repo icon path. Required: there is deliberately no default.
	CrowbarHome func() (string, error)
	// FetchAvatarBytes downloads the GitHub owner avatar bytes for a repo with
	// no local icon. Defaults to avatar.FetchOwnerAvatarBytes when nil.
	FetchAvatarBytes AvatarBytesFetcher
}

// ImportUsecase imports a directory tree as a Project: it creates the
// Project row, discovers repos, persists a Repository per repo, and adopts each
// existing git worktree as a Workspace row.
type ImportUsecase interface {
	Import(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
	// Create is the lightweight variant of Import: it validates the path and
	// persists only the Project row, under id (the caller's; "" mints one).
	// Repo discovery is left to the project-level welcome screen.
	Create(
		ctx context.Context,
		id string,
		name string,
		path string,
	) (domain.Project, error)
	// ImportRepo imports one repo under a persisted project: the Repository row, its
	// icon, a Workspace per existing worktree and per protected branch (each reported
	// to progress). It returns the created Repository.
	ImportRepo(
		ctx context.Context,
		in RepoImport,
		progress RepoProgress,
	) (domain.Repository, error)
	// CheckRepoImportable returns ErrRepoAlreadyImported when another project already
	// has repoPath. ImportRepo enforces it too; the add-repo endpoint runs it first so
	// the refusal answers the request.
	CheckRepoImportable(
		ctx context.Context,
		projectID string,
		repoPath string,
	) error
	// CreateHome creates project's home workspace under its deterministic id.
	// Import does it before the project row is saved; boot does it for a
	// project an older version saved without one.
	CreateHome(
		ctx context.Context,
		project domain.Project,
	) (domain.Workspace, error)
	// SetOwningChats wires the chat side every created workspace is minted under. It is
	// a setter because the chat usecase is built later; until then creates refuse.
	SetOwningChats(chats OwningChats)
}

type projectImport struct {
	deps        ImportDeps
	owningChats OwningChats
}

// SetOwningChats implements ImportUsecase.
func (u *projectImport) SetOwningChats(chats OwningChats) {
	u.owningChats = chats
}

// NewImport builds an ImportUsecase from its dependencies.
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
		// Bulk discovery has no per-repo user name — derive each from its folder.
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

// ImportRepo loads the owning project by id then runs the shared per-repo import
// for a single repo path, returning the created Repository.
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

// loggedProgress is bulk discovery's progress: no operation follows it, so a
// branch that failed is logged as the skipped repo beside it is.
type loggedProgress struct {
	ctx context.Context
}

func (l loggedProgress) Workspace(id, branch string, err error) {
	if err != nil {
		slog.WarnContext(l.ctx, "project import: protected branch has no workspace", "workspace", id, "branch", branch, "error", err)
	}
}

// saveProjectWithHome creates the project's home workspace before saving the row:
// saving announces the project and clients ask for its home at once (no lazy mint).
// A row that fails to save takes its home back out.
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
		// The tombstone's reactor forgets the home's chats and Node row too.
		if dErr := u.deps.Workspaces.Delete(ctx, home.ID); dErr != nil {
			slog.ErrorContext(ctx, "project import: roll back the home workspace",
				"workspace_id", home.ID, "err", dErr)
		}
		return domain.Project{}, fmt.Errorf("save project: %w", err)
	}
	return project, nil
}

// CreateHome implements ImportUsecase: the project's home workspace, rooted at
// the project's own path, owned by a chat and placed like any other.
func (u *projectImport) CreateHome(
	ctx context.Context,
	project domain.Project,
) (domain.Workspace, error) {
	home, err := u.createOwnedWorkspace(ctx, workspace.CreateInput{
		// Deterministic, so a project can never get two homes.
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
