// Package folder owns the sidebar's organisation layer: the Folder rows a user
// files workspaces into, and the dense sibling order both row kinds share.
//
// Folders touch no git. Nothing here takes the per-clone git mutex, shells out,
// or reads a worktree — a folder is a name and two edges — so a reorder never
// queues behind a fetch.
//
// Every operation reads the repo's rows ONCE, plans the whole change in memory,
// and then writes only the rows that actually moved. That is not merely an
// optimisation: the workspace read model is an asynchronous projection, so a
// re-read taken between a write and the renumber that follows it can still be
// serving the pre-write list. Planning from a single snapshot removes the race
// rather than papering over it with a barrier.
package folder

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the folder-table surface this usecase needs: fetch one row by id,
// list a repo's rows without materialising the whole table, persist, remove.
type Store interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Folder, error)
	FindWhere(
		ctx context.Context,
		match domain.Folder,
	) ([]domain.Folder, error)
	Save(
		ctx context.Context,
		folder domain.Folder,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// Workspaces is the workspace surface this usecase needs. It reads the repo's
// rows because workspaces and folders share one sibling space, and it writes
// only the sidebar placement — never the fork lineage.
type Workspaces interface {
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	SetPlacement(
		ctx context.Context,
		id string,
		folderID string,
		order int,
	) (domain.Workspace, error)
}

// CreateInput carries the fields needed to create a folder. ParentID is a
// workspace id, another folder's id, or "" for the repo root; the new folder is
// appended at the end of that sibling space.
type CreateInput struct {
	ID        string
	ProjectID string
	RepoID    string
	ParentID  string
	Name      string
}

// MoveInput is a partial folder placement change: a nil field is left as it is,
// so a reorder within one parent and a move to another are the same call. A move
// with no Order appends to the end of the destination.
type MoveInput struct {
	ParentID *string
	Order    *int
}

// PlaceInput is a partial workspace placement change, mirroring MoveInput.
// FolderID is meaningful only on a fork root; see ErrForkChainSplit.
type PlaceInput struct {
	FolderID *string
	Order    *int
}

// Usecase owns folder CRUD and the dense sibling order shared by folders and
// workspaces. Every mutation leaves the affected levels renumbered 0..n-1.
type Usecase interface {
	// Create appends a new folder to the end of its parent's sibling space and
	// densifies that level. It returns the new folder plus every OTHER folder the
	// densify shifted, so the caller broadcasts the whole change rather than one
	// row of it — the shifted rows' orders are otherwise stale in every client
	// cache until the next reconnect. Shifted WORKSPACE rows need no such
	// handling: their write is an aggregate command, and the hub projection
	// broadcasts every one.
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.Folder, []domain.Folder, error)
	// Rename sets a folder's display name, leaving its placement untouched.
	Rename(
		ctx context.Context,
		id string,
		name string,
	) (domain.Folder, error)
	// Move re-parents and/or reorders a folder, densifying both the level it left
	// and the level it joined. It returns the moved folder plus every other
	// folder those two densifies shifted.
	Move(
		ctx context.Context,
		id string,
		in MoveInput,
	) (domain.Folder, []domain.Folder, error)
	// Delete removes a folder and REPARENTS what it held to the folder's own
	// parent. It never cascades: a folder holds no worktrees, so deleting the
	// workspaces filed under it would destroy work the user only meant to unfile.
	// It returns every folder row the reparent and densify wrote.
	Delete(
		ctx context.Context,
		id string,
	) ([]domain.Folder, error)
	// ListInRepo returns one repo's folders.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Folder, error)
	// PlaceWorkspace files a workspace under a folder (or back at the repo root)
	// and reorders it within that level. It writes no git and no lineage. It
	// returns the placed workspace plus every folder the densify shifted.
	PlaceWorkspace(
		ctx context.Context,
		projectID string,
		repoID string,
		wsID string,
		in PlaceInput,
	) (domain.Workspace, []domain.Folder, error)
	// NextSlot returns the index a row joining container would take — the end of
	// that sibling space. A create has to ask for one: folders and fork-root
	// workspaces share a level, so a row that keeps the zero value collides with
	// whatever already holds slot 0 and surfaces at the top of a level it should
	// have joined at the end.
	NextSlot(
		ctx context.Context,
		projectID string,
		repoID string,
		container string,
	) (int, error)
}

type folderUsecase struct {
	folders    Store
	workspaces Workspaces
}

// New builds the folder usecase over the folders table and the workspace
// repository. The workspace handle is required because the two row kinds share
// one sibling space: renumbering a level without it would leave folders and
// workspaces holding independent, colliding indices.
func New(
	folders Store,
	workspaces Workspaces,
) Usecase {
	return &folderUsecase{folders: folders, workspaces: workspaces}
}

func (u *folderUsecase) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Folder, error) {
	rows, err := u.folders.FindWhere(ctx, domain.Folder{ProjectID: projectID, RepoID: repoID})
	if err != nil {
		return nil, fmt.Errorf("folder: list in repo: %w", err)
	}
	return rows, nil
}

func (u *folderUsecase) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Folder, []domain.Folder, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return domain.Folder{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, in.ProjectID, in.RepoID)
	if err != nil {
		return domain.Folder{}, nil, err
	}
	if cErr := u.checkContainer(ctx, snapshot, in.ParentID); cErr != nil {
		return domain.Folder{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	target := len(snapshot.members(in.ParentID))
	snapshot.add(domain.Folder{
		ID:        id,
		RepoID:    in.RepoID,
		ProjectID: in.ProjectID,
		ParentID:  in.ParentID,
		Name:      name,
		Order:     target,
	})
	snapshot.reorder(in.ParentID, id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Folder{}, nil, err
	}
	return *snapshot.folder(id), without(written, id), nil
}

func (u *folderUsecase) NextSlot(
	ctx context.Context,
	projectID string,
	repoID string,
	container string,
) (int, error) {
	snapshot, err := u.snapshot(ctx, projectID, repoID)
	if err != nil {
		return 0, err
	}
	return len(snapshot.members(container)), nil
}

func (u *folderUsecase) Rename(
	ctx context.Context,
	id string,
	name string,
) (domain.Folder, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.Folder{}, err
	}
	current, err := u.load(ctx, id)
	if err != nil {
		return domain.Folder{}, err
	}
	current.Name = clean
	if err := u.folders.Save(ctx, current); err != nil {
		return domain.Folder{}, fmt.Errorf("folder: rename %s: save: %w", id, err)
	}
	return current, nil
}

func (u *folderUsecase) Move(
	ctx context.Context,
	id string,
	in MoveInput,
) (domain.Folder, []domain.Folder, error) {
	current, err := u.load(ctx, id)
	if err != nil {
		return domain.Folder{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, current.ProjectID, current.RepoID)
	if err != nil {
		return domain.Folder{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkMove(ctx, snapshot, current, destination); mErr != nil {
		return domain.Folder{}, nil, mErr
	}
	target := placementTarget(in.Order, snapshot, current.ParentID, destination, id)
	snapshot.setFolderParent(id, destination)
	snapshot.reorder(destination, id, target)
	if destination != current.ParentID {
		snapshot.reorder(current.ParentID, "", -1)
	}
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Folder{}, nil, err
	}
	return *snapshot.folder(id), without(written, id), nil
}

func (u *folderUsecase) Delete(
	ctx context.Context,
	id string,
) ([]domain.Folder, error) {
	current, err := u.load(ctx, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := u.snapshot(ctx, current.ProjectID, current.RepoID)
	if err != nil {
		return nil, err
	}
	if err := u.folders.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("folder: delete %s: %w", id, err)
	}
	folderDest, workspaceDest := snapshot.reparentChildren(id, current.ParentID)
	snapshot.reorder(folderDest, "", -1)
	if workspaceDest != folderDest {
		snapshot.reorder(workspaceDest, "", -1)
	}
	return u.persist(ctx, snapshot)
}

func (u *folderUsecase) PlaceWorkspace(
	ctx context.Context,
	projectID string,
	repoID string,
	wsID string,
	in PlaceInput,
) (domain.Workspace, []domain.Folder, error) {
	snapshot, err := u.snapshot(ctx, projectID, repoID)
	if err != nil {
		return domain.Workspace{}, nil, err
	}
	located := snapshot.workspace(wsID)
	if located == nil {
		return domain.Workspace{}, nil, fmt.Errorf("folder: place workspace %s: %w", wsID, apperr.ErrNotFound)
	}
	current := *located
	destination, err := u.resolvePlacement(ctx, snapshot, current, in.FolderID)
	if err != nil {
		return domain.Workspace{}, nil, err
	}
	origin := containerOf(current)
	// The folder a workspace names and the sibling space it is ordered within are
	// not the same thing: a workspace WITH a fork parent renders under that
	// parent, so it is ordered among its fork siblings and its folder stays "".
	// Only a fork root is ordered within the folder it names.
	container := current.ParentID
	if container == "" {
		container = destination
	}
	target := placementTarget(in.Order, snapshot, origin, container, wsID)
	snapshot.setWorkspaceFolder(wsID, destination)
	snapshot.reorder(container, wsID, target)
	if container != origin {
		snapshot.reorder(origin, "", -1)
	}
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Workspace{}, nil, err
	}
	return *snapshot.workspace(wsID), written, nil
}

// resolvePlacement returns the container a workspace should end up in and
// refuses the two placements the tree cannot render: filing a forked child away
// from its fork parent, and filing a workspace into a folder that hangs off its
// own subtree.
func (u *folderUsecase) resolvePlacement(
	ctx context.Context,
	snapshot *treeSnapshot,
	current domain.Workspace,
	requested *string,
) (string, error) {
	if requested == nil {
		return current.FolderID, nil
	}
	if *requested == "" {
		return "", nil
	}
	if current.ParentID != "" {
		return "", fmt.Errorf("folder: place workspace %s: %w", current.ID, ErrForkChainSplit)
	}
	if err := u.checkFolderTarget(ctx, snapshot, current, *requested); err != nil {
		return "", err
	}
	if snapshot.reaches(*requested, current.ID) {
		return "", fmt.Errorf("folder: place workspace %s under %s: %w", current.ID, *requested, ErrFolderCycle)
	}
	return *requested, nil
}

// checkMove refuses a folder move onto a container that does not exist in the
// repo, belongs to another repo, or lies inside the folder's own subtree.
func (u *folderUsecase) checkMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	current domain.Folder,
	destination string,
) error {
	if destination == current.ID {
		return fmt.Errorf("folder: move %s onto itself: %w", current.ID, ErrFolderCycle)
	}
	if err := u.checkContainer(ctx, snapshot, destination); err != nil {
		return err
	}
	if snapshot.reaches(destination, current.ID) {
		return fmt.Errorf("folder: move %s under %s: %w", current.ID, destination, ErrFolderCycle)
	}
	return nil
}

// checkContainer validates a parent id: "" is the repo root, and anything else
// must be one of this repo's folders or workspaces. A row that exists under a
// DIFFERENT repo is reported as a cross-repo edge rather than as a missing row,
// because the two are fixed in different ways.
func (u *folderUsecase) checkContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	if snapshot.folder(parentID) != nil || snapshot.workspace(parentID) != nil {
		return nil
	}
	elsewhere, err := u.folders.FindByKey(ctx, parentID)
	if err != nil {
		return fmt.Errorf("folder: resolve parent %s: %w", parentID, err)
	}
	if elsewhere != nil {
		return fmt.Errorf("folder: parent %s is in another repository: %w", parentID, ErrFolderCrossRepo)
	}
	return fmt.Errorf("folder: parent %s: %w", parentID, apperr.ErrNotFound)
}

// checkFolderTarget validates that a workspace's requested folder exists and
// belongs to the workspace's own repo.
func (u *folderUsecase) checkFolderTarget(
	ctx context.Context,
	snapshot *treeSnapshot,
	current domain.Workspace,
	folderID string,
) error {
	if snapshot.folder(folderID) != nil {
		return nil
	}
	elsewhere, err := u.folders.FindByKey(ctx, folderID)
	if err != nil {
		return fmt.Errorf("folder: resolve %s: %w", folderID, err)
	}
	if elsewhere == nil {
		return fmt.Errorf("folder: %s: %w", folderID, apperr.ErrNotFound)
	}
	return fmt.Errorf(
		"folder: %s belongs to %s/%s, not %s/%s: %w",
		folderID, elsewhere.ProjectID, elsewhere.RepoID, current.ProjectID, current.RepoID, ErrFolderCrossRepo,
	)
}

func (u *folderUsecase) load(
	ctx context.Context,
	id string,
) (domain.Folder, error) {
	row, err := u.folders.FindByKey(ctx, id)
	if err != nil {
		return domain.Folder{}, fmt.Errorf("folder: get %s: %w", id, err)
	}
	if row == nil {
		return domain.Folder{}, fmt.Errorf("folder: %s: %w", id, apperr.ErrNotFound)
	}
	return *row, nil
}

// snapshot reads one repo's folders and workspaces as of a single moment. Both
// reads are repo-scoped: the folder query is pushed down to SQL, and the
// workspace read model serves the repo slice natively.
func (u *folderUsecase) snapshot(
	ctx context.Context,
	projectID string,
	repoID string,
) (*treeSnapshot, error) {
	folders, err := u.folders.FindWhere(ctx, domain.Folder{ProjectID: projectID, RepoID: repoID})
	if err != nil {
		return nil, fmt.Errorf("folder: snapshot: folders: %w", err)
	}
	workspaces, err := u.workspaces.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("folder: snapshot: workspaces: %w", err)
	}
	return newTreeSnapshot(folders, workspaces), nil
}

// persist writes exactly the rows the plan touched and returns the FOLDER rows
// among them, which the caller has to broadcast itself. The workspace rows need
// no such handling: their write is an aggregate command, so the hub projection
// broadcasts each one on the way through.
func (u *folderUsecase) persist(
	ctx context.Context,
	snapshot *treeSnapshot,
) ([]domain.Folder, error) {
	written := make([]domain.Folder, 0, len(snapshot.dirty))
	for _, id := range snapshot.dirtyIDs() {
		row, err := u.writeRow(ctx, snapshot, id)
		if err != nil {
			return nil, err
		}
		if row != nil {
			written = append(written, *row)
		}
	}
	return written, nil
}

func (u *folderUsecase) writeRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.Folder, error) {
	if row := snapshot.folder(id); row != nil {
		if err := u.folders.Save(ctx, *row); err != nil {
			return nil, fmt.Errorf("folder: save %s: %w", id, err)
		}
		return row, nil
	}
	row := snapshot.workspace(id)
	if row == nil {
		return nil, nil
	}
	if _, err := u.workspaces.SetPlacement(ctx, id, row.FolderID, row.Order); err != nil {
		return nil, fmt.Errorf("folder: place workspace %s: %w", id, err)
	}
	return nil, nil
}

// without drops the subject row from a written set, leaving the collateral the
// caller broadcasts alongside it.
func without(
	rows []domain.Folder,
	id string,
) []domain.Folder {
	out := make([]domain.Folder, 0, len(rows))
	for _, row := range rows {
		if row.ID != id {
			out = append(out, row)
		}
	}
	return out
}

// placementTarget resolves the index a moved row should land at: the caller's
// explicit request, its current index when it is only being re-parented in
// place, or the end of the destination when it arrives from elsewhere.
func placementTarget(
	requested *int,
	snapshot *treeSnapshot,
	origin string,
	destination string,
	id string,
) int {
	if requested != nil {
		return *requested
	}
	if origin == destination {
		return snapshot.indexOf(destination, id)
	}
	return len(snapshot.members(destination))
}

func cleanName(
	name string,
) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", ErrFolderNameRequired
	}
	return clean, nil
}
