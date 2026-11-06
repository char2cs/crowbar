// Package tree owns the sidebar's organisation layer: folder rows, where every row hangs, and the
// one dense sibling order all row kinds share within a ParentID. Deleting a folder promotes its
// contents (it holds no conversation); deleting a chat takes its subtree (threads read its turns).
// Every operation reads once, plans in memory and writes only moved rows: the chat read model is
// an async projection, so a re-read between writes could serve the pre-write list.
package tree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type chatFolderUsecase struct {
	chats      Chats
	agent      Agent
	busy       Busy
	workspaces WorkspaceGitStatus
	reaper     WorkspaceReaper
	holders    WorkspaceHolders
	// folders and nodes are the folder/chat placement surface for every scope.
	folders Folders
	nodes   Nodes
}

// New builds the tree usecase. Every port is required: reaper so a chat delete cannot strand the worktree
// it owned, holders so it cannot tear down one that surviving chats still use; agent is the only thing
// that can erase or mint a chat.
func New(
	chats Chats,
	agent Agent,
	busy Busy,
	workspaces WorkspaceGitStatus,
	reaper WorkspaceReaper,
	holders WorkspaceHolders,
	folders Folders,
	nodes Nodes,
) Usecase {
	return &chatFolderUsecase{
		chats:      chats,
		agent:      agent,
		busy:       busy,
		workspaces: workspaces,
		reaper:     reaper,
		holders:    holders,
		folders:    folders,
		nodes:      nodes,
	}
}

// ListInRepo returns repoID's own folder rows — "" for project home, a real repo id otherwise, the
// same convention Folder.RepoID is stored under.
func (u *chatFolderUsecase) ListInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	return u.listFolders(ctx, func(f dtree.Folder) bool { return f.RepoID == repoID })
}

// ListInHome implements Usecase. A home folder written before HomeID
// existed is adopted on this read: by the home its rows already attribute it
// to, else by the first home that lists it — never by every home at once.
func (u *chatFolderUsecase) ListInHome(
	ctx context.Context,
	homeID string,
) ([]domain.Chat, error) {
	return u.listFolders(ctx, func(f dtree.Folder) bool {
		if f.RepoID == "" && f.HomeID == "" {
			f = u.adoptHomeFolder(ctx, f, homeID)
		}
		return f.InHome(homeID)
	})
}

// adoptHomeFolder stamps a legacy home folder with the home it belongs to:
// the one its ancestors or members already name, or homeID when nothing does.
func (u *chatFolderUsecase) adoptHomeFolder(
	ctx context.Context,
	f dtree.Folder,
	homeID string,
) dtree.Folder {
	home := u.homeOfFolder(ctx, f.ID, homeID, map[string]bool{})
	if home == "" {
		home = homeID
	}
	f.HomeID = home
	if err := u.folders.Save(ctx, f); err != nil {
		slog.WarnContext(ctx, "agent chat folder: adopt legacy home folder",
			"folder_id", f.ID, "home_id", home, "err", err)
	}
	return f
}

// homeOfFolder derives a home folder's home from the rows around it: a
// chat above or beneath it names its workspace, a folder names its home, a
// repo beneath it names its own project's home — whoever is listing.
func (u *chatFolderUsecase) homeOfFolder(
	ctx context.Context,
	id string,
	homeID string,
	seen map[string]bool,
) string {
	seen[id] = true
	if n, err := u.nodes.GetNode(ctx, id); err == nil && n.ParentID != "" {
		if home := u.homeOfRow(ctx, n.ParentID, homeID, seen); home != "" {
			return home
		}
	}
	children, err := u.nodes.ListByParent(ctx, id)
	if err != nil {
		return ""
	}
	for _, child := range children {
		if home := u.homeOfRow(ctx, child.ID, homeID, seen); home != "" {
			return home
		}
	}
	return ""
}

func (u *chatFolderUsecase) homeOfRow(
	ctx context.Context,
	id string,
	homeID string,
	seen map[string]bool,
) string {
	if seen[id] {
		return ""
	}
	if c, err := u.chats.Get(ctx, id); err == nil {
		return c.WorkspaceID
	}
	if f, err := u.folders.FindByKey(ctx, id); err == nil && f != nil {
		if f.HomeID != "" {
			return f.HomeID
		}
		return u.homeOfFolder(ctx, f.ID, homeID, seen)
	}
	if home, err := u.workspaces.HomeOfRepo(ctx, id); err == nil {
		return home
	}
	return ""
}

func (u *chatFolderUsecase) listFolders(
	ctx context.Context,
	keep func(dtree.Folder) bool,
) ([]domain.Chat, error) {
	all, err := u.folders.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list folders: %w", err)
	}
	out := make([]domain.Chat, 0, len(all))
	for _, f := range all {
		if !keep(f) {
			continue
		}
		row := homeFolderView(f, dtree.Node{})
		if n, nerr := u.nodes.GetNode(ctx, f.ID); nerr == nil {
			row = homeFolderView(f, n)
		}
		out = append(out, row)
	}
	return out, nil
}

// FolderScope implements Usecase.
func (u *chatFolderUsecase) FolderScope(
	ctx context.Context,
	id string,
) (dtree.Folder, error) {
	f, err := u.folders.FindByKey(ctx, id)
	if err != nil {
		return dtree.Folder{}, fmt.Errorf("agent chat folder: %s: %w", id, err)
	}
	if f == nil {
		return dtree.Folder{}, fmt.Errorf("agent chat folder: %s: %w", id, problem.ErrNotFound)
	}
	return *f, nil
}

// Create mints a new folder, home- or repo-scoped: a dtree.Folder row for identity and a dtree.Node row
// for position, placed through the same snapshot/plan/persist densify every other placement runs.
func (u *chatFolderUsecase) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Chat, []domain.Chat, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	folder := dtree.Folder{Name: name, RepoID: in.RepoID, HomeID: in.HomeID}
	snapshot, err := u.globalSnapshotIn(ctx, domain.Chat{}, u.scopeForFolder(ctx, folder))
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if cErr := u.checkFolderContainer(ctx, snapshot, in.RepoID, in.ParentID); cErr != nil {
		return domain.Chat{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	folder.ID = id
	if err := u.folders.Save(ctx, folder); err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: create %s: %w", id, err)
	}
	target := snapshot.plan.NextSlot(snapshot.canonical(in.ParentID))
	snapshot.add(domain.Chat{
		ID:       id,
		Type:     domain.ChatTypeFolder,
		Title:    name,
		RepoID:   in.RepoID,
		ParentID: in.ParentID,
		Order:    target,
	})
	snapshot.homeIDs[id] = true
	snapshot.freshIDs[id] = true
	snapshot.plan.Reorder(snapshot.canonical(in.ParentID), id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, u.discardFolder(ctx, id, err)
	}
	return *snapshot.placedRow(id), without(written, id), nil
}

// discardFolder rolls back a folder whose create failed after minting it. Forget tolerates an unknown
// id, since persist may have failed before the Node row was written; a cleanup failure never replaces
// the cause the user is told about.
func (u *chatFolderUsecase) discardFolder(
	ctx context.Context,
	id string,
	cause error,
) error {
	if err := u.folders.Delete(ctx, id); err != nil {
		return fmt.Errorf("%w (and cleanup failed: %v)", cause, err)
	}
	if err := u.nodes.Forget(ctx, id); err != nil {
		return fmt.Errorf("%w (and node cleanup failed: %v)", cause, err)
	}
	return cause
}

// Rename renames the folder id names in the Folders store, the only place a folder's identity lives;
// an id that names no folder (a chat id included) is problem.ErrNotFound.
func (u *chatFolderUsecase) Rename(
	ctx context.Context,
	id string,
	name string,
) (domain.Chat, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.Chat{}, err
	}
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: %w", id, ferr)
	}
	if f == nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: %s: %w", id, problem.ErrNotFound)
	}
	f.Name = clean
	if err := u.folders.Save(ctx, *f); err != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: save: %w", f.ID, err)
	}
	// A folder with no position row yet sits at its scope's root.
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil && !errors.Is(err, dtree.ErrUnknownNode) {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: position: %w", f.ID, err)
	}
	return homeFolderView(*f, n), nil
}

// Move re-parents and/or reorders the folder id names, refusing while any chat in its subtree is working.
func (u *chatFolderUsecase) Move(
	ctx context.Context,
	id string,
	in MoveInput,
) (domain.Chat, []domain.Chat, error) {
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: move %s: %w", id, ferr)
	}
	if f == nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: %s: %w", id, problem.ErrNotFound)
	}
	// A folder minted before Node rows existed has none; the plan marks it
	// fresh and this move mints it at the decided slot.
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil && problem.CodeOf(err) != problem.NotFound {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: move %s: read node: %w", id, err)
	}
	current := homeFolderView(*f, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if err := u.ensureWorkspaceAnchor(ctx, destination); err != nil {
		return domain.Chat{}, nil, err
	}
	if mErr := u.checkFolderMove(ctx, snapshot, f.RepoID, f.ID, destination); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(ctx, snapshot.subtreeIDs(f.ID), u.busy); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, f.ID, current.ParentID, destination, in.Order, false)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(f.ID), without(written, f.ID), nil
}

// Delete erases the folder id names, promoting what it held to its own parent (a folder holds no
// conversation, so nothing cascades) and closing up the level it sat in.
func (u *chatFolderUsecase) Delete(
	ctx context.Context,
	id string,
) ([]domain.Chat, error) {
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", id, ferr)
	}
	if f == nil {
		return nil, fmt.Errorf("agent chat folder: %s: %w", id, problem.ErrNotFound)
	}
	n, nErr := u.nodes.GetNode(ctx, f.ID)
	current := homeFolderView(*f, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return nil, err
	}
	if wErr := guardNotWorking(ctx, snapshot.subtreeIDs(f.ID), u.busy); wErr != nil {
		return nil, wErr
	}
	if err := u.folders.Delete(ctx, f.ID); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", f.ID, err)
	}
	if nErr == nil {
		if err := u.nodes.Forget(ctx, f.ID); err != nil {
			return nil, fmt.Errorf("agent chat folder: delete %s: node: %w", f.ID, err)
		}
	}
	snapshot.plan.Reparent(f.ID, snapshot.canonical(current.ParentID))
	snapshot.drop(f.ID)
	snapshot.plan.Reorder(snapshot.canonical(current.ParentID), "", -1)
	return u.persist(ctx, snapshot)
}
