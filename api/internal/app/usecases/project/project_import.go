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
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
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
}

// ImportGitEngine is the git surface the import usecase consumes. Import does
// not merely read: it detaches the repo home off a protected branch and
// provisions one managed worktree per protected branch, so it needs the full
// worktree-mutation surface (all satisfied by the engine wired in the container).
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
	// FastForwardBranch fetches origin/<branch> and fast-forwards the local
	// branch ref to match, so the worktree checked out from it starts up to date.
	FastForwardBranch(
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
	// WorktreePrune reaps worktree registrations whose on-disk directory is gone
	// (`git worktree prune`), so a branch held only by a deleted worktree dir is
	// freed before provisioning (the rm -rf ~/.crowbar case). Satisfied by the
	// container's engine (spec §5/B4).
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
	) error
	// RevParse resolves a rev (e.g. a branch name) to a commit SHA, recorded as a
	// managed worktree's fork point.
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
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
	// Repository so the caller can broadcast its DTO (§14 Step 3). name is the
	// user-supplied repository name from the add-repo form; an empty name derives
	// it from filepath.Base(repoPath).
	ImportRepo(
		ctx context.Context,
		projectID string,
		name string,
		repoPath string,
	) (domain.Repository, error)
	// CheckRepoImportable reports whether repoPath may be imported under
	// projectID, returning ErrRepoAlreadyImported (wrapped with the owning
	// project's name) when another project already has that folder. ImportRepo
	// enforces the same rule, but the add-repo endpoint answers 202 and imports
	// in the background — a refusal raised there would reach the user as a
	// timed-out wait for an entity that is never coming, so the endpoint runs
	// this first and fails the request synchronously instead.
	CheckRepoImportable(
		ctx context.Context,
		projectID string,
		repoPath string,
	) error
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
		_ = u.deps.Projects.Delete(ctx, project.ID) // best-effort: don't mask the original error
		return domain.Project{}, fmt.Errorf("project create: home workspace: %w", err)
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
		_ = u.deps.Projects.Delete(ctx, project.ID) // best-effort: don't mask the original error
		return domain.Project{}, fmt.Errorf("project import: home workspace: %w", err)
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
		if _, err := u.importOneRepo(ctx, project, repoPath, ""); err != nil {
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
	name string,
	repoPath string,
) (domain.Repository, error) {
	project, err := u.deps.Projects.FindByKey(ctx, projectID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("import repo: load project: %w", err)
	}
	if project == nil {
		return domain.Repository{}, fmt.Errorf("import repo: project %q not found", projectID)
	}
	return u.importOneRepo(ctx, *project, repoPath, name)
}

func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	repoPath string,
	suppliedName string,
) (domain.Repository, error) {
	// Dedup: re-adding a folder already imported under this project is a no-op
	// (return the existing repo), not a duplicate row. Without this, a re-add would
	// create a second Repository + a branchless duplicate home whose protected
	// worktrees all fail (their branches are already checked out by the first import).
	if existing, ok := u.existingRepo(ctx, project.ID, repoPath); ok {
		return existing, nil
	}
	// The same folder under a DIFFERENT project is not a dedup — it is a refusal.
	// Bulk discovery reaches this too: importRepos logs and skips a repo that
	// errors, which is exactly right for a folder another project already owns.
	if err := u.CheckRepoImportable(ctx, project.ID, repoPath); err != nil {
		return domain.Repository{}, err
	}
	// The user-supplied name (add-repo form) wins; bulk discovery passes "" and
	// falls back to the folder name.
	name := strings.TrimSpace(suppliedName)
	if name == "" {
		name = filepath.Base(repoPath)
	}
	repoID := uuid.NewString()
	runner := u.deps.RefRunner(repoPath)
	remoteURL := gitRemoteURL(repoPath)
	repo := domain.Repository{
		ID:        repoID,
		ProjectID: project.ID,
		Name:      name,
		Path:      repoPath,
		// The on-disk slug is seeded HERE, once, from the path — never from the
		// display name above, which the rename endpoint may change at any time
		// while every already-derived worktree stays where it is.
		PathSlug:      worktreepath.SeedPathSlug(remoteURL, repoPath),
		DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarHasIcon: u.writeRepoIcon(ctx, project.ID, repoID, repoPath),
		RemoteURL:     remoteURL,
	}
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return domain.Repository{}, fmt.Errorf("project import: save repository: %w", err)
	}
	// Roll back the repo row if HOME ADOPTION fails. A repository with no
	// workspaces is unnavigable (workspaces are the UI's unit) and unusable; never
	// leave one persisted. Once the home workspace exists the repo is navigable and
	// must be kept, so committed is set before the best-effort protected-branch
	// managed worktrees (whose per-branch failure must NOT roll back a repo that
	// already has its home workspace).
	committed := false
	defer func() {
		if !committed {
			_ = u.deps.Repos.Delete(ctx, repo.ID)
		}
	}()
	// Resolve protected branches once: it decides whether the home must detach
	// off a protected branch and which branches get their own managed worktree. A
	// provider failure is soft — import the repo home alone rather than failing the
	// whole repo (the provider falls back to a default set when its CLI is absent).
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		slog.WarnContext(ctx, "project import: protected branches unavailable; importing repo home only",
			"repo", repo.Name, "error", err)
		protected = nil
	} else if len(protected) == 0 && repo.DefaultBranch != "" {
		// A repo with no protected branches whatsoever — no GitHub protection
		// rules, or a provider that succeeded with an empty set — still needs its
		// base branch locked (the "Base branch (locked)" UX contract). Seed the
		// resolved default branch so it is materialised as a locked, Crowbar-managed
		// worktree like any protected branch. A provider ERROR stays home-only
		// above: when detection failed we cannot assert which branch is the base.
		protected = []string{repo.DefaultBranch}
	}
	// Adopt the repo home as the special default workspace (Crowbar never runs git
	// on it). If it sits on a protected branch, it is detached to HEAD first so
	// that branch is free for its own managed worktree. This is the one essential
	// workspace — its failure rolls the repo back.
	if err := u.adoptRepoHome(ctx, repo); err != nil {
		return domain.Repository{}, err
	}
	committed = true
	// Every protected branch gets its own Crowbar-managed worktree (a locked
	// branch-tree root the user bases work off). Best-effort per branch.
	u.provisionProtectedWorktrees(ctx, repo, protected)
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
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil { //nolint:gosec // G301: repo icon dir lives under the user's own ~/.crowbar home; 0755 intentional
		slog.WarnContext(ctx, "project import: create repo icon dir failed", "error", err)
		return false
	}
	if err := os.WriteFile(iconPath, data, 0o644); err != nil { //nolint:gosec // G306: repo icon is a non-sensitive display asset; 0644 intentional
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
		if data, err := os.ReadFile(src); err == nil { //nolint:gosec // G304: src is a repo-icon path discovered under the user's own imported repo, not external input
			return data
		}
	}
	data, _, err := u.deps.FetchAvatarBytes(ctx, repoPath)
	if err != nil {
		return nil
	}
	return data
}

// adoptRepoHome adopts the repo's main worktree (repo.Path) as the special
// default workspace, IN PLACE on whatever branch it currently sits on. Crowbar
// never runs git on this directory — it is the user-facing "home" that carries
// chats/threads — so it is created non-protected. The home is NO LONGER
// force-detached off a protected branch: that branch is materialised as a
// placeholder by provisionProtectedWorktrees (the single owner), whose
// holder.Resolve returns held-by-home for it, and freed only later with user
// consent via the Detach-holder op (spec §3.5). The home is the one essential
// workspace; its failure rolls the repo back.
func (u *projectImport) adoptRepoHome(
	ctx context.Context,
	repo domain.Repository,
) error {
	worktrees, err := u.deps.Git.WorktreeList(ctx, repo.Path)
	if err != nil {
		return fmt.Errorf("project import: list worktrees: %w", err)
	}
	in := workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       mainWorktreeBranch(worktrees, repo.Path),
		WorktreePath: repo.Path,
		IsDefault:    true,
		// ForkPointSha stays empty and Protected stays false: the home is the base
		// the branch tree hangs off, and Crowbar does not operate on it.
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("project import: adopt repo home: %w", err)
	}
	return nil
}

// CheckRepoImportable refuses repoPath when a DIFFERENT project has already
// imported that folder (symlink-resolving path compare, so the same repo reached
// through a symlinked root is still caught).
//
// Two projects cannot share a folder: git checks a branch out in one worktree at
// a time, so the second project's import claims nothing — every protected branch
// its sibling already holds lands as a placeholder, and the repository sits there
// looking imported while being unable to manage a single branch. Refusing up
// front is the honest answer, and it names the project that has the folder so the
// user knows where it went.
//
// A FindAll failure degrades to "importable", matching existingRepo: a read
// failure must not block a legitimate import.
func (u *projectImport) CheckRepoImportable(
	ctx context.Context,
	projectID string,
	repoPath string,
) error {
	repos, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return nil
	}
	for _, r := range repos {
		if r.ProjectID == projectID || !samePath(r.Path, repoPath) {
			continue
		}
		owner := u.projectName(ctx, r.ProjectID)
		if owner == "" {
			return ErrRepoAlreadyImported
		}
		return fmt.Errorf("%w in the project %q", ErrRepoAlreadyImported, owner)
	}
	return nil
}

// projectName resolves a project's display name for the already-imported
// message, returning "" when it cannot be read — the refusal still stands, it
// just loses the pointer to where the folder already lives.
func (u *projectImport) projectName(
	ctx context.Context,
	projectID string,
) string {
	p, err := u.deps.Projects.FindByKey(ctx, projectID)
	if err != nil || p == nil {
		return ""
	}
	return p.Name
}

// existingRepo returns an already-imported repository at repoPath under projectID
// (symlink-resolving path compare), so a re-add/re-import is a no-op. A FindAll
// failure is treated as "not found" — proceed with the import rather than block it.
func (u *projectImport) existingRepo(
	ctx context.Context,
	projectID string,
	repoPath string,
) (domain.Repository, bool) {
	repos, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return domain.Repository{}, false
	}
	for _, r := range repos {
		if r.ProjectID == projectID && samePath(r.Path, repoPath) {
			return r, true
		}
	}
	return domain.Repository{}, false
}

// mainWorktreeBranch returns the branch checked out in the repo's main worktree
// (the entry whose path is repo.Path), or "" when it is detached or absent.
func mainWorktreeBranch(
	worktrees []gitengine.WorktreeEntry,
	repoPath string,
) string {
	for _, wt := range worktrees {
		if samePath(wt.Path, repoPath) {
			return wt.Branch
		}
	}
	return ""
}

// provisionProtectedWorktrees gives every protected branch its own
// Crowbar-managed git worktree — a locked branch-tree root the user bases work
// off. Best-effort per branch: a failure (branch absent, branch already checked
// out elsewhere, git error) is logged and skipped so it never rolls back a repo
// that already has its home workspace.
func (u *projectImport) provisionProtectedWorktrees(
	ctx context.Context,
	repo domain.Repository,
	protected []string,
) {
	if len(protected) == 0 {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		slog.WarnContext(ctx, "project import: crowbar home unavailable; skipping protected worktrees",
			"repo", repo.Name, "error", err)
		return
	}
	for _, branch := range protected {
		if err := u.provisionProtectedBranchWorktree(ctx, repo, branch, home); err != nil {
			slog.WarnContext(ctx, "project import: skip protected branch managed worktree",
				"repo", repo.Name, "branch", branch, "error", err)
		}
	}
}

// provisionProtectedBranchWorktree materialises one protected branch. It first
// resolves who holds the branch (pruning dead registrations): ANY live holder
// yields a PLACEHOLDER row (locked, empty WorktreePath, HeldByPath = holder),
// and only a free branch gets its own managed worktree. This is the SINGLE owner
// of placeholder creation (spec §3.2/§3.5).
//
// A holder under the crowbar home (holder.HeldByManaged) used to be skipped as
// "already represented by a managed workspace — never double-provision". That
// judgement was made from the FILESYSTEM, not from this repo's rows, and it can
// never be about THIS repo: importOneRepo returns early for an already-imported
// folder, so the repo aggregate reaching here is always brand new and owns no
// workspace yet. The holder is somebody else's — in practice a managed worktree
// that outlived the repo it was created for, since removing a repository deletes
// its row but leaves its LOCKED worktrees on disk — and a branch git will not let
// this repo check out is held, not represented. Skipping it dropped the branch
// with no row AND no warning (the skip was the one silent path in provisioning),
// so re-adding such a folder produced a repo with no locked workspace and nothing
// to explain why.
//
// The other way here — two projects importing one folder — is now refused
// outright by CheckRepoImportable, so it can no longer reach this code.
func (u *projectImport) provisionProtectedBranchWorktree(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	crowbarHome string,
) error {
	outcome, err := holder.Resolve(ctx, u.deps.Git, repo.Path, branch, crowbarHome)
	if err != nil {
		return fmt.Errorf("resolve holder for %q: %w", branch, err)
	}
	if outcome.Kind != holder.Free {
		return u.createPlaceholderWorkspace(ctx, repo, branch, outcome.HeldByPath)
	}
	// Free: provision the managed worktree at its human-readable derived path
	// <home>/projects/<project>/<slug>/<branch> (spec §3.9).
	wsID := uuid.NewString()
	slug := worktreepath.RemoteSlug(repo)
	path, err := worktreepath.Derive(crowbarHome, repo.ProjectID, slug, branch)
	if err != nil {
		return fmt.Errorf("derive worktree path for %q: %w", branch, err)
	}
	siblings, err := siblingWorktreePaths(crowbarHome, repo.ProjectID, slug)
	if err != nil {
		return fmt.Errorf("scan sibling worktrees for %q: %w", branch, err)
	}
	if clashErr := worktreepath.DetectClash(siblings, path); clashErr != nil {
		return fmt.Errorf("worktree path clash for %q: %w", branch, clashErr)
	}
	startSha, err := u.addProtectedWorktree(ctx, repo, branch, path)
	if err != nil {
		return err
	}
	in := workspace.CreateInput{
		ID:           wsID,
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		Protected:    true,
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		// The row failed after the worktree was created on disk — remove the
		// orphaned worktree so a later retry can recreate it cleanly.
		if rmErr := u.deps.Git.WorktreeRemove(ctx, repo.Path, path); rmErr != nil {
			slog.WarnContext(ctx, "project import: failed to clean up orphaned worktree",
				"path", path, "error", rmErr)
		}
		return fmt.Errorf("create protected workspace row: %w", err)
	}
	return nil
}

// createPlaceholderWorkspace persists a locked, worktree-less row recording the
// live holder of a protected branch, so the branch is never silently dropped and
// the user gets a visible, retryable surface (spec §3.3). No LastError is written
// — the FE reconstructs the reason from HeldByPath (spec §4/B7).
func (u *projectImport) createPlaceholderWorkspace(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	heldByPath string,
) error {
	in := workspace.CreateInput{
		ID:         uuid.NewString(),
		RepoID:     repo.ID,
		ProjectID:  repo.ProjectID,
		Branch:     branch,
		Protected:  true, // seeds locked; keeps every protection guard for free (B1)
		HeldByPath: heldByPath,
		// WorktreePath + ForkPointSha stay empty — this is the placeholder signal.
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("create placeholder workspace for %q: %w", branch, err)
	}
	return nil
}

// addProtectedWorktree checks branch out into a fresh worktree at path and
// returns the branch tip SHA (the workspace's fork point). The parent fetch is
// BEST-EFFORT (matching addWorktree): a refused fetch (a dead reg still
// "holding" the branch, or an offline remote) must NOT skip the branch — branch
// from the local tip instead (spec §3.2). WorktreeAdd already prunes-and-retries
// the stale "already used by worktree" conflict internally.
func (u *projectImport) addProtectedWorktree(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	path string,
) (string, error) {
	if exists, err := u.deps.Git.RemoteBranchExists(ctx, repo.Path, branch); err == nil && exists {
		if err := u.deps.Git.FastForwardBranch(ctx, repo.Path, branch); err != nil {
			slog.WarnContext(ctx, "project import: could not fast-forward protected branch; branching from local tip",
				"repo", repo.Name, "branch", branch, "error", err)
		}
	}
	if err := u.deps.Git.WorktreeAdd(ctx, repo.Path, path, branch); err != nil {
		return "", fmt.Errorf("add worktree for protected branch %q: %w", branch, err)
	}
	// Resolve the BRANCH head explicitly: `git rev-parse <name>` resolves a tag of
	// the same name before the branch, which would record a wrong fork point.
	sha, err := u.deps.Git.RevParse(ctx, repo.Path, "refs/heads/"+branch)
	if err != nil {
		return "", nil // fork point is non-essential; the worktree is valid
	}
	return sha, nil
}

// samePath reports whether two filesystem paths refer to the same location,
// resolving symlinks first so a repo imported under a symlinked root still
// matches the path git reports for its main worktree. This matters because git
// worktree list emits the fully-resolved path (e.g. macOS /var -> /private/var,
// or a symlinked home / network mount), while repo.Path is the path as imported;
// a naive string compare would then never flag the main worktree as default.
// Falls back to a lexical clean when a path cannot be resolved (e.g. it no
// longer exists on disk).
func samePath(a, b string) bool {
	return resolvePath(a) == resolvePath(b)
}

func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
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

// gitRemoteURL returns the origin remote URL for the repo at path, or ""
// on any failure so callers can fall back gracefully.
func gitRemoteURL(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output() //nolint:gosec // G204: fixed git subcommand; only the repo dir is variable, passed as -C <path> (no shell)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// siblingWorktreePaths lists the existing branch-leaf worktrees under a repo's
// derived slug directory, so a managed-worktree create can reject a
// case-insensitive path clash (spec §3.9). A missing slug directory yields none.
func siblingWorktreePaths(
	crowbarHome string,
	projectID string,
	slug string,
) ([]string, error) {
	parent := filepath.Join(crowbarHome, "projects", projectID, slug)
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(parent, entry.Name()))
	}
	return paths, nil
}
