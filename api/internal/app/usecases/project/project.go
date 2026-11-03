package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Usecase is the read + roll-up surface for the Project entity.
type Usecase interface {
	// List returns every project stored in the GORM table.
	List(
		ctx context.Context,
	) ([]domain.Project, error)

	// Get returns the project with the given id, or an error when not found.
	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)

	// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up
	// the repo's projectId, updates the project row, and logs any failure
	// without returning it (00 §5.1).
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)

	// UpdateRepo applies a partial repository update in one load-mutate-save. A rename touches the
	// label only: PathSlug, the on-disk identity, is left as loaded. It answers the decided placement
	// and every Node the write shifted, so the caller announces settled state, not an async projection.
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in RepoUpdate,
	) error
	// Reorder sets a project's sidebar index and densifies the whole list, so
	// every project ends up holding a distinct 0..n-1 slot.
	Reorder(
		ctx context.Context,
		projectID string,
		order int,
	) (domain.Project, error)

	// Update applies a partial project update in one load-mutate-save (problem.ErrNotFound for an
	// unknown id). A rename touches the label only: Project.Path is what disk is keyed on.
	Update(
		ctx context.Context,
		projectID string,
		in Update,
	) (domain.Project, error)
	// NextHomeSlot answers the first free index at projectID's home root —
	// where a freshly imported repo's Node is minted, so the level stays
	// dense from birth instead of every new row tying at 0.
	NextHomeSlot(
		ctx context.Context,
		projectID string,
	) (int, error)
}

// Update is a partial project update: a nil field is left as it is. The icon fields are one
// three-state choice (emoji > image > default glyph); setting one without clearing the others
// leaves a project showing the icon it was told to replace.
type Update struct {
	Name          *string
	AvatarEmoji   *string
	AvatarHasIcon *bool
	// BumpAvatarVersion moves the DTO's cache-busting ?v= param. Set it whenever
	// new bytes have been written behind the stable icon URL.
	BumpAvatarVersion bool
}

// RepoUpdate is a partial repository update: a nil field is left as it is.
// ProjectID may only name the repo's own project (see refuseProjectMove);
// FolderID/Order re-file the repo among its home siblings.
type RepoUpdate struct {
	Name      *string
	ProjectID *string
	Order     *int
	FolderID  *string
}

// HomeWorkspaces answers which workspace is project home, so a root-level repo placement can scope
// home chats (which carry no project id) to this project. The workspace repository satisfies it:
// the workspace usecase is built after, and depends on, this package.
type HomeWorkspaces interface {
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
}

// Folders confirms a FolderID names a genuine project-home folder (RepoID == "") before
// validateRepoFolder writes it — the same rule checkFolderContainer enforces for chat folders.
type Folders interface {
	FindByKey(ctx context.Context, id string) (*tree.Folder, error)
}

// HomeChats answers project membership for CHAT-kind home siblings: tree.Node carries no project id,
// so only the chat aggregate can scope them. Satisfied by the chat repository (repos.AgentChat).
type HomeChats interface {
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
}

// NodePlacements is the Node surface a repo's own placement needs: minting its row (Create), the
// sibling reads a densify or placement renumbers, the SetOrder/SetPlacement writes, and Forget to
// undo an import's Create. Every home-scope sibling (chat, folder, repo) shares this one surface.
type NodePlacements interface {
	Create(
		ctx context.Context,
		id string,
		kind tree.Kind,
		parentID string,
		order int,
	) (tree.Node, error)
	GetNode(
		ctx context.Context,
		id string,
	) (tree.Node, error)
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]tree.Node, error)
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	SetPlacement(
		ctx context.Context,
		id string,
		parentID string,
		order int,
	) error
	// Forget purges a Node row outright, only to unwind a Create a repo import is rolling back —
	// never to delete a live row.
	Forget(
		ctx context.Context,
		id string,
	) error
}

type projectUsecase struct {
	projects   store.Store[domain.Project, string]
	repos      store.ScopedStore[domain.Repository, string]
	workspaces HomeWorkspaces
	folders    Folders
	nodes      NodePlacements
	homeChats  HomeChats
}

// New builds a Usecase. A nil homeChats degrades rather than fails: the bare-root densify cannot
// narrow CHAT-kind siblings to this project and includes them all.
func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces HomeWorkspaces,
	folders Folders,
	nodes NodePlacements,
	homeChats HomeChats,
) Usecase {
	return &projectUsecase{
		projects:   projects,
		repos:      repos,
		workspaces: workspaces,
		folders:    folders,
		nodes:      nodes,
		homeChats:  homeChats,
	}
}

// List returns every project stored in the GORM table.
func (u *projectUsecase) List(
	ctx context.Context,
) ([]domain.Project, error) {
	list, err := u.projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}
	return list, nil
}

// Get returns the project with the given id, or an error when not found.
func (u *projectUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	p, err := u.projects.FindByKey(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: get: %w", err)
	}
	if p == nil {
		return domain.Project{}, fmt.Errorf("project: get: id %s: %w", id, problem.ErrNotFound)
	}
	return *p, nil
}

// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up the
// repo's projectId, updates the project row, and logs any failure without
// returning it (00 §5.1).
func (u *projectUsecase) TouchProjectActivity(
	ctx context.Context,
	repoID string,
	now time.Time,
) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		slog.WarnContext(ctx, "project: touch activity: repo not found", "repoID", repoID)
		return
	}
	project, err := u.projects.FindByKey(ctx, repo.ProjectID)
	if err != nil || project == nil {
		slog.WarnContext(ctx, "project: touch activity: project not found", "projectID", repo.ProjectID)
		return
	}
	project.LastActivity = now
	if err := u.projects.Save(ctx, *project); err != nil {
		slog.ErrorContext(ctx, "project: touch activity: save failed", "projectID", project.ID, "err", err)
	}
}

// UpdateRepo applies a partial repository update in one load-mutate-save. A rename refreshes the
// generated avatar. The row is saved whole so PathSlug, the on-disk identity every worktree lives
// under, travels untouched: changing it would split the repo's tree across two names.
func (u *projectUsecase) UpdateRepo(
	ctx context.Context,
	repoID string,
	in RepoUpdate,
) error {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return fmt.Errorf("project: update repo: %w", err)
	}
	if repo == nil {
		return fmt.Errorf("project: update repo: id %s: %w", repoID, problem.ErrNotFound)
	}
	if err := refuseProjectMove(*repo, in.ProjectID); err != nil {
		return err
	}
	subject, err := u.getRepoNode(ctx, repoID)
	if err != nil {
		return fmt.Errorf("project: update repo: node: %w", err)
	}
	if in.Name != nil {
		repo.Name = *in.Name
		repo.AvatarLabel = avatar.Label(*in.Name)
		repo.AvatarColor = avatar.Color(*in.Name)
	}
	originFolder := subject.ParentID
	targetFolder, err := u.resolveTargetFolder(ctx, in, originFolder)
	if err != nil {
		return err
	}
	if err := u.repos.Save(ctx, *repo); err != nil {
		return fmt.Errorf("project: update repo: save: %w", err)
	}
	if _, err := u.applyRepoPlacement(ctx, *repo, targetFolder, subject, in.Order); err != nil {
		return err
	}
	// The folder the repo LEFT still has a gap where its row used to sit and
	// needs closing. A plain reorder within the same container is covered by
	// the densify above; this only fires for an actual move.
	if originFolder != targetFolder {
		if _, err := u.densifyHomeLevel(ctx, repo.ProjectID, originFolder, repoID); err != nil {
			return err
		}
	}
	return nil
}

// getRepoNode reads repoID's own Node row. Every repo mints one at import
// (importOneRepo), so a missing row is an error, never a zero-value to mint
// on the first drag — all data is on the Node model (spec §6.3).
func (u *projectUsecase) getRepoNode(
	ctx context.Context,
	repoID string,
) (tree.Node, error) {
	n, err := u.nodes.GetNode(ctx, repoID)
	if err != nil {
		return tree.Node{}, err
	}
	return n, nil
}

// resolveTargetFolder validates and resolves the repo's target folder from
// the RepoUpdate: originFolder unchanged when in.FolderID is nil (nothing
// asked to move it), else the validated new value.
func (u *projectUsecase) resolveTargetFolder(
	ctx context.Context,
	in RepoUpdate,
	originFolder string,
) (string, error) {
	if in.FolderID == nil {
		return originFolder, nil
	}
	if err := u.validateRepoFolder(ctx, *in.FolderID); err != nil {
		return "", err
	}
	return *in.FolderID, nil
}

// applyRepoPlacement picks the pass an update needs: an explicit order (a drag) needs the
// cross-aggregate placement, since densifyRepos alone clamps it wrong beside home chats/folders;
// a move with no order only re-sorts the repo rows by their current (order, id).
func (u *projectUsecase) applyRepoPlacement(
	ctx context.Context,
	repo domain.Repository,
	targetFolder string,
	subject tree.Node,
	order *int,
) ([]tree.Node, error) {
	if order != nil {
		return u.placeRepoAmongHomeSiblings(ctx, repo.ProjectID, targetFolder, subject, *order)
	}
	return u.densifyRepos(ctx, repo.ProjectID, targetFolder, &subject)
}

// validateRepoFolder accepts "" (project home root) or a tree.Folder with no repo scope; a
// repo-internal folder is refused, as a repo cannot contain itself. It does not check the folder
// belongs to this project: the home tree has no project anchor yet.
func (u *projectUsecase) validateRepoFolder(
	ctx context.Context,
	folderID string,
) error {
	if folderID == "" {
		return nil
	}
	folder, err := u.folders.FindByKey(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: update repo: folder %s: %w", folderID, err)
	}
	if folder == nil || folder.RepoID != "" {
		return fmt.Errorf(
			"project: update repo: %s is not a project-home folder: %w", folderID, problem.ErrInvalidArgument,
		)
	}
	return nil
}

// refuseProjectMove answers ErrConflict for an update naming another project. A repo's project is
// fixed at import: its worktrees, storage and entity dirs are keyed by it and live agents run there,
// so moving rows would move none of that. Remove and re-import instead.
func refuseProjectMove(
	repo domain.Repository,
	projectID *string,
) error {
	if projectID == nil || *projectID == repo.ProjectID {
		return nil
	}
	return fmt.Errorf(
		"project: update repo: a repository cannot change projects; remove it and import it into %s: %w",
		*projectID, problem.ErrConflict,
	)
}

// repoIDSet answers the repo ids of projectID: tree.Node carries no project id, so membership comes
// from Repository.ProjectID intersected with what Node.ListByParent returns.
func (u *projectUsecase) repoIDSet(
	ctx context.Context,
	projectID string,
) (map[string]bool, error) {
	rows, err := u.repos.FindWhere(ctx, domain.Repository{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list: %w", err)
	}
	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.ID] = true
	}
	return ids, nil
}

// densifyRepos renumbers folderID's repo siblings in projectID to 0..n-1, writing only moved rows.
// subject is included authoritatively, since Node's read model may not have folded its reparent
// yet; its write is SetPlacement when its old parent differs, every other row's is SetOrder.
func (u *projectUsecase) densifyRepos(
	ctx context.Context,
	projectID string,
	folderID string,
	subject *tree.Node,
) ([]tree.Node, error) {
	memberIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return nil, err
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	rows := densifyRows(memberIDs, nodes, subject)
	subjectID := ""
	if subject != nil {
		subjectID = subject.ID
	}
	slots := nodeIndex(rows)
	written := make(map[string]bool, len(rows))
	var decided []tree.Node
	for _, moved := range place(slots, subjectID, nil) {
		row := rows[moved.at]
		written[row.ID] = true
		decided = append(decided, tree.Node{ID: row.ID, Kind: row.Kind, ParentID: folderID, Order: moved.order})
		reparenting := subject != nil && row.ID == subject.ID && subject.ParentID != folderID
		if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
			return nil, err
		}
	}
	if subject != nil {
		reparented := subject.ParentID != folderID
		n, err := u.ensureSubjectWritten(ctx, slots, subject.ID, reparented, folderID, nil, written)
		if err != nil {
			return nil, err
		}
		if n != nil {
			decided = append(decided, *n)
		}
	}
	return decided, nil
}

// densifyRows lists densifyRepos' candidates: projectID's nodes in folderID's sibling space, with
// subject's stale copy replaced by the authoritative one.
func densifyRows(
	memberIDs map[string]bool,
	nodes []tree.Node,
	subject *tree.Node,
) []tree.Node {
	rows := make([]tree.Node, 0, len(nodes)+1)
	for _, n := range nodes {
		if !memberIDs[n.ID] {
			continue
		}
		if subject != nil && n.ID == subject.ID {
			continue // superseded by subject below, which is authoritative
		}
		rows = append(rows, n)
	}
	if subject != nil {
		rows = append(rows, *subject)
	}
	return rows
}

// writeNode issues the one write a densify or placement owes a Node row: SetPlacement when it
// reparents, SetOrder otherwise.
func (u *projectUsecase) writeNode(
	ctx context.Context,
	id string,
	folderID string,
	order int,
	reparenting bool,
) error {
	if reparenting {
		if err := u.nodes.SetPlacement(ctx, id, folderID, order); err != nil {
			return fmt.Errorf("project: reorder repos: place %s: %w", id, err)
		}
		return nil
	}
	if err := u.nodes.SetOrder(ctx, id, order); err != nil {
		return fmt.Errorf("project: reorder repos: save %s: %w", id, err)
	}
	return nil
}

// mintNode creates the Node row for a repo that has none yet (no backfill ships), at the resolved
// (folderID, order): SetOrder/SetPlacement rightly refuse a row that was never Created.
func (u *projectUsecase) mintNode(
	ctx context.Context,
	id string,
	kind tree.Kind,
	folderID string,
	order int,
) error {
	if _, err := u.nodes.Create(ctx, id, kind, folderID, order); err != nil {
		return fmt.Errorf("project: reorder repos: mint %s: %w", id, err)
	}
	return nil
}

// ensureSubjectWritten lands a reparenting subject in its new container even when place() saw no
// order change to make. slots is the original (pre-sort) slot list.
func (u *projectUsecase) ensureSubjectWritten(
	ctx context.Context,
	slots []slot,
	subjectID string,
	reparented bool,
	folderID string,
	target *int,
	written map[string]bool,
) (*tree.Node, error) {
	if written[subjectID] {
		return nil, nil
	}
	i := finalIndexOf(slots, subjectID, target)
	if !reparented || i < 0 {
		return nil, nil // place() correctly found no write needed
	}
	if err := u.nodes.SetPlacement(ctx, subjectID, folderID, i); err != nil {
		return nil, fmt.Errorf("project: reorder repos: place %s: %w", subjectID, err)
	}
	return &tree.Node{ID: subjectID, Kind: tree.KindRepo, ParentID: folderID, Order: i}, nil
}

// placeRepoAmongHomeSiblings gives subject the asked Order within (projectID, folderID), shifting
// repos, home chats and home folders alike; subject is included authoritatively (see densifyRepos),
// and a row with no Node yet is minted where it lands.
func (u *projectUsecase) placeRepoAmongHomeSiblings(
	ctx context.Context,
	projectID string,
	folderID string,
	subject tree.Node,
	target int,
) ([]tree.Node, error) {
	siblings, err := u.homeLevel(ctx, projectID, folderID, subject.ID)
	if err != nil {
		return nil, err
	}
	rows := append(siblings, homeRow{Node: subject})
	slots := homeIndex(rows)
	reparented := subject.ParentID != folderID
	decided, written, err := u.writeHomeLevel(ctx, rows, place(slots, subject.ID, &target),
		folderID, subject.ID, reparented)
	if err != nil {
		return nil, err
	}
	n, err := u.ensureSubjectWritten(ctx, slots, subject.ID, reparented, folderID, &target, written)
	if err != nil {
		return nil, err
	}
	if n != nil {
		decided = append(decided, *n)
	}
	return decided, nil
}

// densifyHomeLevel closes the gap a departed repo left over the level's full member set; exclude is
// the row known to have left even if Node's read model has not folded it yet.
func (u *projectUsecase) densifyHomeLevel(
	ctx context.Context,
	projectID string,
	folderID string,
	exclude string,
) ([]tree.Node, error) {
	rows, err := u.homeLevel(ctx, projectID, folderID, exclude)
	if err != nil {
		return nil, err
	}
	decided, _, err := u.writeHomeLevel(ctx, rows, place(homeIndex(rows), "", nil),
		folderID, "", false)
	return decided, err
}

// writeHomeLevel writes every move a home-level pass decided, mints the
// fresh rows the level touched; the Tree's own announce publishes each move.
func (u *projectUsecase) writeHomeLevel(
	ctx context.Context,
	rows []homeRow,
	moves []move,
	folderID string,
	subjectID string,
	reparented bool,
) ([]tree.Node, map[string]bool, error) {
	written := make(map[string]bool, len(rows))
	var decided []tree.Node
	for _, moved := range moves {
		row := rows[moved.at]
		written[row.ID] = true
		decided = append(decided, tree.Node{ID: row.ID, Kind: row.Kind, ParentID: folderID, Order: moved.order})
		if row.fresh {
			if err := u.mintNode(ctx, row.ID, row.Kind, folderID, moved.order); err != nil {
				return nil, nil, err
			}
		} else {
			reparenting := row.ID == subjectID && reparented
			if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
				return nil, nil, err
			}
		}
	}
	// A legacy root chat whose slot did not move (place reports no change, so
	// its order IS its final index) still becomes Node-backed: the level was
	// touched, and the next read must find every row on one surface.
	for _, row := range rows {
		if !row.fresh || written[row.ID] || row.ID == subjectID {
			continue
		}
		written[row.ID] = true
		if err := u.mintNode(ctx, row.ID, row.Kind, folderID, row.Order); err != nil {
			return nil, nil, err
		}
	}
	return decided, written, nil
}

// Reorder sets a project's sidebar index and densifies the whole list. Projects
// are the sidebar's top level, so the full set IS the sibling space and reading
// all of them is the scope, not an over-read.
func (u *projectUsecase) Reorder(
	ctx context.Context,
	projectID string,
	order int,
) (domain.Project, error) {
	rows, err := u.projects.FindAll(ctx)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: reorder: list: %w", err)
	}
	if !containsID(projectIndex(rows), projectID) {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, problem.ErrNotFound)
	}
	for _, moved := range place(projectIndex(rows), projectID, &order) {
		rows[moved.at].Order = moved.order
		if err := u.projects.Save(ctx, rows[moved.at]); err != nil {
			return domain.Project{}, fmt.Errorf("project: reorder: save %s: %w", rows[moved.at].ID, err)
		}
	}
	updated, err := u.projects.FindByKey(ctx, projectID)
	if err != nil || updated == nil {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, problem.ErrNotFound)
	}
	return *updated, nil
}

// Update applies a partial project update in one load-mutate-save.
func (u *projectUsecase) Update(
	ctx context.Context,
	projectID string,
	in Update,
) (domain.Project, error) {
	row, err := u.projects.FindByKey(ctx, projectID)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: update: load %s: %w", projectID, err)
	}
	if row == nil {
		return domain.Project{}, fmt.Errorf("project: update: id %s: %w", projectID, problem.ErrNotFound)
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return domain.Project{}, fmt.Errorf("project: update: name is empty: %w", problem.ErrInvalidArgument)
		}
		row.Name = name
	}
	if in.AvatarEmoji != nil {
		row.AvatarEmoji = *in.AvatarEmoji
	}
	if in.AvatarHasIcon != nil {
		row.AvatarHasIcon = *in.AvatarHasIcon
	}
	if in.BumpAvatarVersion {
		row.AvatarVersion++
	}
	if err := u.projects.Save(ctx, *row); err != nil {
		return domain.Project{}, fmt.Errorf("project: update: save %s: %w", projectID, err)
	}
	return *row, nil
}
