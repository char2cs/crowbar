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
	folders    Folders
	nodes      Nodes
	moves      moveRuns
}

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

func (u *chatFolderUsecase) ListInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	return u.listFolders(ctx, func(f dtree.Folder) bool { return f.RepoID == repoID })
}

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
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil && !errors.Is(err, dtree.ErrUnknownNode) {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: position: %w", f.ID, err)
	}
	return homeFolderView(*f, n), nil
}

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
