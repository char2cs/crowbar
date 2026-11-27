package tree

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func homeFolderView(
	f dtree.Folder,
	n dtree.Node,
) domain.Chat {
	return domain.Chat{
		ID:       f.ID,
		Type:     domain.ChatTypeFolder,
		RepoID:   f.RepoID,
		Title:    f.Name,
		ParentID: n.ParentID,
		Order:    n.Order,
	}
}

const workspaceAnchorType domain.ChatType = "__workspace_anchor__"

func workspaceAnchorView(
	workspaceID string,
	n dtree.Node,
	createdAt time.Time,
) domain.Chat {
	return domain.Chat{
		ID:          workspaceID,
		Type:        workspaceAnchorType,
		WorkspaceID: workspaceID,
		ParentID:    n.ParentID,
		Order:       n.Order,
		CreatedAt:   createdAt,
	}
}

func (u *chatFolderUsecase) anchorView(
	ctx context.Context,
	workspaceID string,
	n dtree.Node,
) (domain.Chat, error) {
	createdAt, err := u.workspaces.CreatedAtOf(ctx, workspaceID)
	if err != nil && problem.CodeOf(err) != problem.NotFound {
		return domain.Chat{}, fmt.Errorf("agent chat folder: read workspace %s: %w", workspaceID, err)
	}
	return workspaceAnchorView(workspaceID, n, createdAt), nil
}

const nodePhantomType domain.ChatType = "__node_phantom__"

func (u *chatFolderUsecase) correctHomePlacement(
	ctx context.Context,
	row domain.Chat,
) (domain.Chat, error) {
	if row.WorkspaceID == "" {
		return row, nil
	}
	n, err := u.nodes.GetNode(ctx, row.ID)
	if err != nil {
		if row.ParentID != "" && !u.rowExists(ctx, row.ParentID) {
			row.ParentID = ""
		}
		return row, nil
	}
	row.ParentID = n.ParentID
	row.Order = n.Order
	return row, nil
}

func (u *chatFolderUsecase) mergeForest(
	ctx context.Context,
	baseRows []domain.Chat,
	scope forestScope,
	extraSeeds []string,
) (forest, error) {
	byID := make(map[string]int, len(baseRows))
	for i, row := range baseRows {
		byID[row.ID] = i
	}
	aliases := u.levelAliasesOf(ctx, baseRows)
	all, err := u.nodes.ListAll(ctx)
	if err != nil {
		return forest{}, fmt.Errorf("agent chat folder: node forest: %w", err)
	}
	byParent := map[string][]dtree.Node{}
	for _, n := range all {
		byParent[n.ParentID] = append(byParent[n.ParentID], n)
	}
	homeIDs := map[string]bool{}
	nodeSeen := map[string]bool{}
	queried := map[string]bool{}
	queue := make([]string, 0, len(baseRows)+1+len(extraSeeds)+len(aliases))
	queue = append(queue, "")
	for _, row := range baseRows {
		queue = append(queue, row.ID)
	}
	queue = append(queue, extraSeeds...)
	for id := range aliases {
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if queried[parent] {
			continue
		}
		queried[parent] = true
		for _, n := range byParent[parent] {
			if nodeSeen[n.ID] {
				continue
			}
			nodeSeen[n.ID] = true
			included, childID, err := u.mergeHomeNode(ctx, n, byID, &baseRows, scope, aliases)
			if err != nil {
				return forest{}, err
			}
			if included {
				homeIDs[n.ID] = true
			}
			if childID != "" && !queried[childID] {
				queue = append(queue, childID)
			}
		}
	}
	f := forest{rows: baseRows, homeIDs: homeIDs, fresh: map[string]bool{}, aliases: aliases}
	f.addNodelessRepos(scope, nodeSeen)
	if err := u.addNodelessAnchors(ctx, &f, scope, nodeSeen); err != nil {
		return forest{}, err
	}
	f.foreign = u.foreignAtRoot(ctx, f.rows, scope, aliases)
	return f, nil
}

type forest struct {
	rows    []domain.Chat
	homeIDs map[string]bool
	fresh   map[string]bool
	foreign map[string]bool
	aliases levelAliases
}

func (u *chatFolderUsecase) mergeHomeNode(
	ctx context.Context,
	n dtree.Node,
	byID map[string]int,
	baseRows *[]domain.Chat,
	scope forestScope,
	aliases levelAliases,
) (included bool, childID string, err error) {
	if aliases.canonical(n.ParentID) == "" {
		var row *domain.Chat
		if i, ok := byID[n.ID]; ok {
			row = &(*baseRows)[i]
		}
		if !u.rootMember(ctx, n, scope, aliases, row) {
			if n.Kind == dtree.KindWorkspace && n.ID == scope.workspaceID {
				return false, n.ID, nil
			}
			return false, "", nil
		}
	}
	switch n.Kind {
	case dtree.KindFolder:
		f, ferr := u.folders.FindByKey(ctx, n.ID)
		if ferr != nil {
			return false, "", fmt.Errorf("agent chat folder: node forest: folder %s: %w", n.ID, ferr)
		}
		if f == nil {
			return false, "", nil
		}
		*baseRows = append(*baseRows, homeFolderView(*f, n))
		return true, n.ID, nil
	case dtree.KindChat:
		if i, ok := byID[n.ID]; ok {
			(*baseRows)[i].ParentID = n.ParentID
			(*baseRows)[i].Order = n.Order
			return true, n.ID, nil
		}
		byID[n.ID] = len(*baseRows)
		*baseRows = append(*baseRows, domain.Chat{ID: n.ID, ParentID: n.ParentID, Order: n.Order})
		return true, n.ID, nil
	case dtree.KindRepo:
		if !scope.home || (scope.repoMemberIDs != nil && !scope.repoMemberIDs[n.ID]) {
			return false, "", nil
		}
		*baseRows = append(*baseRows, domain.Chat{
			ID: n.ID, Type: nodePhantomType, ParentID: n.ParentID, Order: n.Order,
		})
		return true, "", nil
	case dtree.KindWorkspace:
		included, err := u.mergeAnchorNode(ctx, n, baseRows)
		if problem.CodeOf(err) == problem.NotFound {
			return false, "", nil
		}
		return included, n.ID, err
	}
	return false, "", nil
}

func (u *chatFolderUsecase) mergeAnchorNode(
	ctx context.Context,
	n dtree.Node,
	baseRows *[]domain.Chat,
) (bool, error) {
	renders, err := u.workspaces.RendersAsBranch(ctx, n.ID)
	if err != nil {
		return false, fmt.Errorf("agent chat folder: read workspace %s: %w", n.ID, err)
	}
	if !renders {
		return false, nil
	}
	anchor, err := u.anchorView(ctx, n.ID, n)
	if err != nil {
		return false, err
	}
	*baseRows = append(*baseRows, anchor)
	return true, nil
}

func (u *chatFolderUsecase) homeSnapshotAround(
	ctx context.Context,
	workspaceID string,
	rows []domain.Chat,
	subject domain.Chat,
	seeds ...string,
) (*treeSnapshot, error) {
	scope, err := u.scopeForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	f, err := u.mergeForest(ctx, rows, scope, append([]string{subject.ParentID}, seeds...))
	if err != nil {
		return nil, err
	}
	return buildHomeSnapshot(f, subject, subject.ID != ""), nil
}

func (u *chatFolderUsecase) repoMemberIDsForHome(
	ctx context.Context,
	homeWorkspaceID string,
) map[string]bool {
	ids, err := u.workspaces.RepoIDsForHome(ctx, homeWorkspaceID)
	if err != nil {
		return nil
	}
	return ids
}

func buildHomeSnapshot(
	f forest,
	subject domain.Chat,
	subjectIsHome bool,
) *treeSnapshot {
	_, discovered := f.homeIDs[subject.ID]
	if subjectIsHome {
		f.homeIDs[subject.ID] = true
	}
	delete(f.foreign, subject.ID)
	snap := newTreeSnapshotScoped(corrected(f.rows, subject), f.foreign, f.aliases)
	snap.homeIDs = f.homeIDs
	snap.freshIDs = f.fresh
	if subjectIsHome && !discovered {
		snap.freshIDs[subject.ID] = true
	}
	return snap
}
