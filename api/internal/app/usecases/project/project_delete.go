package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeleteProjectStore is the project persistence surface the delete usecase
// needs: resolve the project row, record its delete intent and outcome, and
// remove it.
type DeleteProjectStore interface {
	FindAll(
		ctx context.Context,
	) ([]domain.Project, error)
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Project, error)
	Save(
		ctx context.Context,
		p domain.Project,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteRepositoryStore is the repository persistence surface the delete
// usecase needs: list every repo row (to filter by project), record a repo's
// delete intent and outcome, and remove rows.
type DeleteRepositoryStore interface {
	FindAll(
		ctx context.Context,
	) ([]domain.Repository, error)
	Save(
		ctx context.Context,
		r domain.Repository,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteWorkspaceRepo is the workspace persistence surface the delete usecase
// needs: list every workspace row and tombstone the ones no repo cascade owns.
type DeleteWorkspaceRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteRepoWorkspaces retires every workspace of one repo through the SAME
// cascade a repo delete takes (hierarchy.DeleteRepoWorkspaces): the git
// teardown that never deletes a branch Crowbar did not create and never forces
// a locked worktree, then the tombstone the delete reactor purges.
type DeleteRepoWorkspaces interface {
	DeleteRepoWorkspaces(
		ctx context.Context,
		repo domain.Repository,
	) error
}

// DeleteNodes drops a deleted entity's sidebar Node row.
type DeleteNodes interface {
	Forget(
		ctx context.Context,
		id string,
	) error
}

// DeleteDeps wires the delete usecase's collaborators.
type DeleteDeps struct {
	Projects       DeleteProjectStore
	Repos          DeleteRepositoryStore
	Workspaces     DeleteWorkspaceRepo
	RepoWorkspaces DeleteRepoWorkspaces
	Nodes          DeleteNodes
	CrowbarHome    func() (string, error)
}

// DeleteUsecase removes a project and everything it owns — and nothing it
// does not.
//
// A delete first records its intent on the row (Deleting), so a failure or a
// crash anywhere after it is never silent: the row stays, carries LastError,
// and Resume re-drives it at boot (spec §7-D, invariant D5).
//
// Its workspaces are retired through the one lifecycle path every delete takes
// (repo cascade → tombstone → delete reactor), so the git rules hold here too:
// no branch Crowbar did not create is deleted, no locked worktree is forced,
// and the user's real repository directories are never touched. Then its repo
// rows and Node rows, the project row, and finally the project's directory —
// minus anything that is not the project's to remove (see removeProjectDir).
type DeleteUsecase interface {
	// BeginDelete records a project's delete intent — Deleting set, LastError
	// cleared — and returns the row as recorded. Delete begins with it too;
	// a caller that answers before the teardown runs calls it first, so the
	// intent is durable and announced before the answer.
	BeginDelete(
		ctx context.Context,
		id string,
	) (domain.Project, error)
	Delete(
		ctx context.Context,
		id string,
	) error
	// BeginRepoDelete is BeginDelete for one repo.
	BeginRepoDelete(
		ctx context.Context,
		repo domain.Repository,
	) (domain.Repository, error)
	// DeleteRepo removes one repo and everything it owns, in the one order a
	// crash cannot corrupt: its workspaces are retired (git teardown, then the
	// tombstones the delete reactor purges) BEFORE its row goes, so no crash can
	// leave workspaces whose repo no longer resolves; then its row, its Node
	// row and its entity directory.
	DeleteRepo(
		ctx context.Context,
		repo domain.Repository,
	) error
	// Resume re-drives every project and repo delete a crash or a failure
	// left unfinished. Per-row failures are recorded on the row, not returned.
	Resume(
		ctx context.Context,
	) error
}

type projectDelete struct {
	deps DeleteDeps
}

// NewDelete builds a DeleteUsecase from its dependencies.
func NewDelete(
	deps DeleteDeps,
) DeleteUsecase {
	return &projectDelete{deps: deps}
}

func (u *projectDelete) BeginDelete(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	p, err := u.deps.Projects.FindByKey(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project delete: find project: %w", err)
	}
	if p == nil {
		return domain.Project{}, fmt.Errorf("project delete: id %s: %w", id, apperr.ErrNotFound)
	}
	p.Deleting, p.LastError = true, ""
	if err := u.deps.Projects.Save(ctx, *p); err != nil {
		return domain.Project{}, fmt.Errorf("project delete: record intent: %w", err)
	}
	return *p, nil
}

func (u *projectDelete) Delete(
	ctx context.Context,
	id string,
) error {
	p, err := u.BeginDelete(ctx, id)
	if err != nil {
		return err
	}
	if err := u.teardownProject(ctx, id); err != nil {
		p.LastError = err.Error()
		if saveErr := u.deps.Projects.Save(ctx, p); saveErr != nil {
			slog.ErrorContext(ctx, "project delete: record failure", "project_id", id, "err", saveErr)
		}
		return err
	}
	return nil
}

// teardownProject retires everything the project owns, then its row and
// directory. Every step is idempotent, so a re-drive converges.
func (u *projectDelete) teardownProject(
	ctx context.Context,
	id string,
) error {
	repos, err := u.projectRepos(ctx, id)
	if err != nil {
		return err
	}
	// Listed BEFORE any row goes: removeProjectDir needs every other project's
	// live paths, and the rows this delete tombstones are filtered out by owner.
	all, err := u.deps.Workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("project delete: list workspaces: %w", err)
	}
	for _, repo := range repos {
		if err := u.DeleteRepo(ctx, repo); err != nil {
			return fmt.Errorf("project delete: %w", err)
		}
	}
	if err := u.deleteRemainingWorkspaces(ctx, id, repos, all); err != nil {
		return err
	}
	if err := u.deps.Projects.Delete(ctx, id); err != nil {
		return fmt.Errorf("project delete: delete project %s: %w", id, err)
	}
	u.removeProjectDir(ctx, id, repos, all)
	return nil
}

func (u *projectDelete) DeleteRepo(
	ctx context.Context,
	repo domain.Repository,
) error {
	repo, err := u.BeginRepoDelete(ctx, repo)
	if err != nil {
		return err
	}
	if err := u.teardownRepo(ctx, repo); err != nil {
		repo.LastError = err.Error()
		if saveErr := u.deps.Repos.Save(ctx, repo); saveErr != nil {
			slog.ErrorContext(ctx, "delete repo: record failure", "repo", repo.ID, "err", saveErr)
		}
		return err
	}
	return nil
}

func (u *projectDelete) BeginRepoDelete(
	ctx context.Context,
	repo domain.Repository,
) (domain.Repository, error) {
	repo.Deleting, repo.LastError = true, ""
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return domain.Repository{}, fmt.Errorf("repo %s: record delete intent: %w", repo.ID, err)
	}
	return repo, nil
}

func (u *projectDelete) teardownRepo(
	ctx context.Context,
	repo domain.Repository,
) error {
	if err := u.deps.RepoWorkspaces.DeleteRepoWorkspaces(ctx, repo); err != nil {
		return fmt.Errorf("repo %s workspaces: %w", repo.ID, err)
	}
	if err := u.deps.Repos.Delete(ctx, repo.ID); err != nil {
		return fmt.Errorf("delete repo %s: %w", repo.ID, err)
	}
	u.forgetNode(ctx, repo.ID)
	u.removeRepoDir(ctx, repo)
	return nil
}

func (u *projectDelete) Resume(
	ctx context.Context,
) error {
	projects, err := u.deps.Projects.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("resume deletes: list projects: %w", err)
	}
	deleting := map[string]bool{}
	for _, p := range projects {
		if !p.Deleting {
			continue
		}
		deleting[p.ID] = true
		if err := u.Delete(ctx, p.ID); err != nil {
			slog.ErrorContext(ctx, "resume deletes: project delete stopped again", "project_id", p.ID, "err", err)
		}
	}
	repos, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("resume deletes: list repos: %w", err)
	}
	for _, repo := range repos {
		if !repo.Deleting || deleting[repo.ProjectID] {
			continue
		}
		if err := u.DeleteRepo(ctx, repo); err != nil {
			slog.ErrorContext(ctx, "resume deletes: repo delete stopped again", "repo", repo.ID, "err", err)
		}
	}
	return nil
}

// removeRepoDir removes the repo's entity directory (icon, storages) under the
// crowbar home. Best-effort: the row is already gone.
func (u *projectDelete) removeRepoDir(
	ctx context.Context,
	repo domain.Repository,
) {
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	dir := worktreepath.RepoDir(home, repo.ProjectID, repo.ID)
	if err := removeTreeKeeping(dir, worktreepath.IsLiveCheckout); err != nil {
		slog.ErrorContext(ctx, "delete repo: remove entity dir", "repo", repo.ID, "dir", dir, "err", err)
	}
}

// deleteRemainingWorkspaces tombstones the project's rows no repo cascade took
// — its home workspace, which belongs to the project rather than to a repo.
// The delete reactor purges them like any other tombstone.
func (u *projectDelete) deleteRemainingWorkspaces(
	ctx context.Context,
	projectID string,
	repos map[string]domain.Repository,
	all []domain.Workspace,
) error {
	for _, ws := range all {
		// A repo's rows were the repo cascade's; a row of a repo that now
		// belongs to another project is that project's, whatever its own
		// (denormalised) ProjectID still says.
		if ws.RepoID != "" || ws.ProjectID != projectID || ws.Status == domain.WorkspaceStatusDeleted {
			continue
		}
		if err := u.deps.Workspaces.Delete(ctx, ws.ID); err != nil && !alreadyGone(err) {
			return fmt.Errorf("project delete: delete workspace %s: %w", ws.ID, err)
		}
	}
	return nil
}

func (u *projectDelete) forgetNode(
	ctx context.Context,
	id string,
) {
	if u.deps.Nodes == nil {
		return
	}
	if err := u.deps.Nodes.Forget(ctx, id); err != nil && !alreadyGone(err) {
		slog.WarnContext(ctx, "project delete: forget node row", "id", id, "err", err)
	}
}

// alreadyGone reports an error meaning the thing being removed no longer
// exists — for a delete, success.
func alreadyGone(err error) bool {
	return errors.Is(err, apperr.ErrNotFound) || errors.Is(err, asynxModels.ErrValidation)
}

// removeProjectDir removes ~/.crowbar/projects/<P> — except what is not the
// project's to remove (spec §3 P0-3, invariant D6):
//
//   - a live git checkout (a directory holding `.git`): a worktree git refused
//     to remove, such as a protected one with uncommitted work. Deleting it
//     would destroy that work and strand its registration in the user's repo.
//   - the path of any live workspace that belongs to ANOTHER project. A repo
//     moved to project B (before moves were refused) keeps its worktrees
//     under projects/A; an rm -rf of projects/A used to wipe them.
//
// Everything else goes, and a directory is removed only once it is empty, so
// whatever is kept keeps its parents. Workspace roots the delete reactor is
// purging concurrently are simply found already gone. Best-effort: the rows are
// deleted by now, so a failure is logged, never returned.
func (u *projectDelete) removeProjectDir(
	ctx context.Context,
	projectID string,
	repos map[string]domain.Repository,
	all []domain.Workspace,
) {
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	dir := worktreepath.ProjectDir(home, projectID)
	// Another project's live workspace keeps its whole root: the worktree and
	// the chats tree beside it.
	foreign := map[string]bool{}
	for _, ws := range all {
		if ownedBy(ws, projectID, repos) || ws.Status == domain.WorkspaceStatusDeleted || ws.WorktreePath == "" {
			continue
		}
		path := filepath.Clean(ws.WorktreePath)
		if filepath.Base(path) == "worktree" {
			path = filepath.Dir(path)
		}
		foreign[path] = true
	}
	keep := func(path string) bool {
		return foreign[path] || worktreepath.IsLiveCheckout(path)
	}
	if err := removeTreeKeeping(dir, keep); err != nil {
		slog.ErrorContext(ctx, "project delete: remove project dir; records already gone, part of the directory left on disk",
			"project_id", projectID, "dir", dir, "err", err)
	}
}

// ownedBy reports whether ws is the project's: a repo's row belongs to the
// project that owns the repo (the repo row is the one owner of that
// assignment); a repo-less row — the project home — to its own ProjectID.
func ownedBy(
	ws domain.Workspace,
	projectID string,
	repos map[string]domain.Repository,
) bool {
	if ws.RepoID == "" {
		return ws.ProjectID == projectID
	}
	_, mine := repos[ws.RepoID]
	return mine
}

// removeTreeKeeping removes dir and everything under it except the paths keep
// claims. A directory is removed only once it is empty, so whatever is kept
// keeps its ancestors. It never follows a symlink.
func removeTreeKeeping(
	dir string,
	keep func(path string) bool,
) error {
	if keep(dir) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			errs = append(errs, removeTreeKeeping(path, keep))
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	// Fails (harmlessly) while something kept is still inside.
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) && !isNotEmpty(dir) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func isNotEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func (u *projectDelete) projectRepos(
	ctx context.Context,
	projectID string,
) (map[string]domain.Repository, error) {
	all, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("project delete: list repos: %w", err)
	}
	owned := make(map[string]domain.Repository)
	for _, repo := range all {
		if repo.ProjectID == projectID {
			owned[repo.ID] = repo
		}
	}
	return owned, nil
}
