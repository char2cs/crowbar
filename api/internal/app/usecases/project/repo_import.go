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

type RepoImport struct {
	ID        string
	ProjectID string
	Name      string
	Path      string
}

type RepoProgress interface {
	Workspace(id, branch string, err error)
}

func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	in RepoImport,
	progress RepoProgress,
) (domain.Repository, error) {
	existing, found, err := u.existingRepo(ctx, project.ID, in.Path)
	if err != nil || found {
		return existing, err
	}
	if err := u.CheckRepoImportable(ctx, project.ID, in.Path); err != nil {
		return domain.Repository{}, err
	}
	repo := u.newRepo(ctx, project.ID, in)
	if err := u.saveRepoWithHome(ctx, repo); err != nil {
		return domain.Repository{}, err
	}
	u.provisionProtectedWorktrees(ctx, repo, u.protectedBranches(ctx, repo), progress)
	return repo, nil
}

func (u *projectImport) newRepo(
	ctx context.Context,
	projectID string,
	in RepoImport,
) domain.Repository {
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
		ID:            id,
		ProjectID:     projectID,
		Name:          name,
		Path:          in.Path,
		PathSlug:      worktreepath.SeedPathSlug(remoteURL, in.Path),
		DefaultBranch: defaultbranch.Resolve(u.deps.RefRunner(in.Path), defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarHasIcon: u.writeRepoIcon(ctx, projectID, id, in.Path),
		RemoteURL:     remoteURL,
	}
}

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
	}
	if _, err := u.createOwnedWorkspace(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("project import: adopt repo home: %w", err)
	}
	return nil
}

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

func gitRemoteURL(ctx context.Context, repoPath string) string {
	out, err := exec.CommandContext(ctx, binpath.Git(), "-C", repoPath, "remote", "get-url", "origin").Output() //nolint:gosec // G204: fixed git subcommand; only the repo dir is variable, passed as -C <path> (no shell)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
