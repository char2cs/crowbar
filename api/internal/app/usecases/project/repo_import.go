package project

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// RepoImport is one repository to add under an existing project. ID is the
// caller's (an operation names the repo before it exists); "" mints one, for
// bulk discovery, which has no caller.
type RepoImport struct {
	ID        string
	ProjectID string
	Name      string
	Path      string
}

// RepoProgress hears how each protected branch's workspace came out: id names
// the workspace, err is why it could not be set up.
type RepoProgress interface {
	Workspace(id, branch string, err error)
}

func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	in RepoImport,
	progress RepoProgress,
) (domain.Repository, error) {
	// Re-adding a folder this project already has returns that repo: a second row
	// would get a branchless home whose protected worktrees all fail.
	existing, found, err := u.existingRepo(ctx, project.ID, in.Path)
	if err != nil || found {
		return existing, err
	}
	// The same folder under a DIFFERENT project is a refusal, not a dedup.
	if err := u.CheckRepoImportable(ctx, project.ID, in.Path); err != nil {
		return domain.Repository{}, err
	}
	repo := u.newRepo(ctx, project.ID, in)
	if err := u.saveRepoWithHome(ctx, repo); err != nil {
		return domain.Repository{}, err
	}
	// Each protected branch gets its own Crowbar-managed worktree; one that
	// fails is reported and the repo, which has its home, is kept.
	u.provisionProtectedWorktrees(ctx, repo, u.protectedBranches(ctx, repo), progress)
	return repo, nil
}

func (u *projectImport) newRepo(
	ctx context.Context,
	projectID string,
	in RepoImport,
) domain.Repository {
	// The add-repo form's name wins; bulk discovery passes "" and gets the folder's.
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = filepath.Base(in.Path)
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	remoteURL := gitRemoteURL(ctx, in.Path)
	return domain.Repository{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		Path:      in.Path,
		// Seeded once from the path, never from the renameable display name.
		PathSlug:      worktreepath.SeedPathSlug(remoteURL, in.Path),
		DefaultBranch: defaultbranch.Resolve(u.deps.RefRunner(in.Path), defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarHasIcon: u.writeRepoIcon(ctx, projectID, id, in.Path),
		RemoteURL:     remoteURL,
	}
}

// saveRepoWithHome saves the repo row, mints its Node row and adopts its home.
// A repo with no home is unnavigable, so a failure before the home exists
// takes the rows back out; a retry mints fresh ones.
func (u *projectImport) saveRepoWithHome(
	ctx context.Context,
	repo domain.Repository,
) error {
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return fmt.Errorf("project import: save repository: %w", err)
	}
	if err := u.placeRepoAndAdoptHome(ctx, repo); err != nil {
		u.rollBackRepo(ctx, repo.ID)
		return err
	}
	return nil
}

func (u *projectImport) placeRepoAndAdoptHome(
	ctx context.Context,
	repo domain.Repository,
) error {
	// Every repo gets its position row at creation, so UpdateRepo never reads a
	// row that was never born; it starts at the end of the project-home root.
	if u.deps.Nodes == nil {
		return ErrNoNodesWired
	}
	slot := 0
	if u.deps.HomeSlots != nil {
		if next, sErr := u.deps.HomeSlots.NextHomeSlot(ctx, repo.ProjectID); sErr == nil {
			slot = next
		}
	}
	if _, err := u.deps.Nodes.Create(ctx, repo.ID, tree.KindRepo, "", slot); err != nil {
		return fmt.Errorf("project import: mint repo node: %w", err)
	}
	return u.adoptRepoHome(ctx, repo)
}

func (u *projectImport) rollBackRepo(
	ctx context.Context,
	repoID string,
) {
	if err := u.deps.Repos.Delete(ctx, repoID); err != nil {
		slog.ErrorContext(ctx, "project import: roll back the repo row", "repo", repoID, "err", err)
	}
	if u.deps.Nodes == nil {
		return
	}
	if err := u.deps.Nodes.Forget(ctx, repoID); err != nil {
		slog.ErrorContext(ctx, "project import: roll back the repo's node row", "repo", repoID, "err", err)
	}
}

// protectedBranches is what gets a locked managed worktree. A provider failure
// imports the home alone, since the base branch cannot be asserted; an empty
// answer still locks the default branch (the "Base branch (locked)" contract).
func (u *projectImport) protectedBranches(
	ctx context.Context,
	repo domain.Repository,
) []string {
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		slog.WarnContext(ctx, "project import: protected branches unavailable; importing repo home only",
			"repo", repo.Name, "error", err)
		return nil
	}
	if len(protected) == 0 && repo.DefaultBranch != "" {
		return []string{repo.DefaultBranch}
	}
	return protected
}

// writeRepoIcon writes the repo icon (local icon first, then a best-effort owner-avatar download) to
// <home>/projects/<P>/<R>/icon and reports whether it did. Failures fall back to a generated avatar.
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
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o700); err != nil {
		slog.WarnContext(ctx, "project import: create repo icon dir failed", "error", err)
		return false
	}
	if err := os.WriteFile(iconPath, data, 0o600); err != nil {
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

// adoptRepoHome adopts repo.Path as the default workspace in place, on its current branch, never running
// git there. A held protected branch becomes a placeholder (provisionProtectedWorktrees), freed only by a
// consented Detach; the home is essential, so its failure rolls the repo back.
func (u *projectImport) adoptRepoHome(
	ctx context.Context,
	repo domain.Repository,
) error {
	worktrees, err := u.deps.Git.WorktreeList(ctx, repo.Path)
	if err != nil {
		return fmt.Errorf("project import: list worktrees: %w", err)
	}
	in := workspace.CreateInput{
		ID:           workspace.RepoHomeID(repo.ID),
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       mainWorktreeBranch(worktrees, repo.Path),
		WorktreePath: repo.Path,
		IsDefault:    true,
		Provisioning: domain.WorkspaceShared,
		// ForkPointSha stays empty and Protected stays false: the home is the base
		// the branch tree hangs off, and Crowbar does not operate on it.
	}
	if _, err := u.createOwnedWorkspace(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("project import: adopt repo home: %w", err)
	}
	return nil
}

// CheckRepoImportable refuses repoPath when a DIFFERENT project already imported it (symlink-resolved):
// git checks a branch out once, so a second project could manage nothing. The error names the owning
// project; a failed read is returned rather than guessed importable.
func (u *projectImport) CheckRepoImportable(
	ctx context.Context,
	projectID string,
	repoPath string,
) error {
	repos, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("project import: list repositories: %w", err)
	}
	for _, r := range repos {
		if r.ProjectID == projectID || !worktreepath.SamePath(r.Path, repoPath) {
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
// (symlink-resolving path compare), so a re-add/re-import is a no-op.
func (u *projectImport) existingRepo(
	ctx context.Context,
	projectID string,
	repoPath string,
) (domain.Repository, bool, error) {
	repos, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return domain.Repository{}, false, fmt.Errorf("project import: list repositories: %w", err)
	}
	for _, r := range repos {
		if r.ProjectID == projectID && worktreepath.SamePath(r.Path, repoPath) {
			return r, true, nil
		}
	}
	return domain.Repository{}, false, nil
}

// mainWorktreeBranch returns the branch checked out in the repo's main worktree
// (the entry whose path is repo.Path), or "" when it is detached or absent.
func mainWorktreeBranch(
	worktrees []gitengine.WorktreeEntry,
	repoPath string,
) string {
	for _, wt := range worktrees {
		if worktreepath.SamePath(wt.Path, repoPath) {
			return wt.Branch
		}
	}
	return ""
}

// gitRemoteURL returns the origin remote URL for the repo at path, or ""
// on any failure so callers can fall back gracefully.
func gitRemoteURL(ctx context.Context, repoPath string) string {
	out, err := exec.CommandContext(ctx, binpath.Git(), "-C", repoPath, "remote", "get-url", "origin").Output() //nolint:gosec // G204: fixed git subcommand; only the repo dir is variable, passed as -C <path> (no shell)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
