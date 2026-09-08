package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The home-scoped (2026-09-08 sidebar-placement-unification Task 5) read
// side: rendering a Folder+Node pair as the Chat-shaped view the rest of this
// package plans and broadcasts against, and mergeHomeForest — the direct
// Node-native replacement for project.go's now-deleted cross-aggregate merge.

// homeFolderView renders a home-scoped domain.Folder+domain.Node pair as the
// same Chat-shaped row every other verb in this package plans and broadcasts
// — the wire contract does not change, only where a home folder's
// identity/position are sourced from. n's zero value is a legitimate answer:
// a folder whose Node row has not been minted yet (this call IS the mint)
// sits at the panel root with Order 0, exactly like a freshly-minted repo or
// chat.
func homeFolderView(
	f domain.Folder,
	n domain.Node,
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

// nodePhantomType marks a repo's own Node row as it rides through this
// package's Chat-shaped treeSnapshot — a repo is neither a folder nor a chat
// (isFolder/isChat must both answer false for it) and is never written back
// through Chats, only Nodes (see mergeHomeForest, writeHomeNode). It is never
// persisted or serialised; the value only has to be distinct from every real
// domain.ChatType.
const nodePhantomType domain.ChatType = "__node_phantom__"

// isHomeWorkspace answers whether workspaceID is a project's home workspace —
// RepoOf resolving "" is the SAME convention repoScopeOf/checkFolderContainer
// already use elsewhere in this package. A bubble ("") is never home: it is
// mid-creation, ownerless (see workspaceSnapshotAround's own doc), and never
// resolves through RepoOf at all.
func (u *chatFolderUsecase) isHomeWorkspace(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	if workspaceID == "" {
		return false, nil
	}
	repoID, err := u.workspaces.RepoOf(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("agent chat folder: resolve workspace %s: %w", workspaceID, err)
	}
	return repoID == "", nil
}

// correctHomePlacement overrides a home-scoped chat row's ParentID/Order with
// its live Node row. Chat.ParentID/.Order are write-once-at-creation for a
// home-scoped chat from this task onward (position authority moves to Node) —
// full cleanup of the now-dead fields is Task 11's job, but a caller reading
// row.ParentID for ANYTHING placement-related (origin container, lineage,
// cwd) needs the live answer NOW, not the frozen one. A row still not yet
// minted onto Node (its very first placement) keeps its Chat-native
// (zero-value) placement, which is what a brand new bubble already carries.
func (u *chatFolderUsecase) correctHomePlacement(
	ctx context.Context,
	row domain.Chat,
) (domain.Chat, error) {
	home, err := u.isHomeWorkspace(ctx, row.WorkspaceID)
	if err != nil || !home {
		return row, nil
	}
	n, err := u.nodes.GetNode(ctx, row.ID)
	if err != nil {
		return row, nil
	}
	row.ParentID = n.ParentID
	row.Order = n.Order
	return row, nil
}

// mergeHomeForest walks the home portion of the Node forest breadth-first
// from the panel root, folding it into baseRows: a real chat row already in
// baseRows has its ParentID/Order corrected to match its live Node row; a
// home folder (Folder.RepoID == "") is appended as a homeFolderView; a repo's
// own Node row is appended as a nodePhantomType placeholder purely so it
// participates in the SAME densify/sibling-count pass as its chat/folder
// siblings — this is the direct replacement for project.go's now-deleted
// cross-aggregate merge: every home-scope sibling now comes from ONE
// Node.ListByParent walk.
//
// repoMemberIDs scopes which repo phantoms are actually included — the SAME
// reason project.go's placeRepoAmongHomeSiblings needs repoIDSet/
// homeChatIDSet (SDD review Critical 2): domain.Node carries no project id
// of its own, so a repo sharing the bare root ("") belongs to SOME project,
// not necessarily the caller's, and appending it unscoped would let a
// densify in one project renumber — and WRITE, via writeHomeNode's
// Nodes.SetOrder/.SetPlacement — another project's repo Node row. nil means
// "do not filter": globalSnapshot/globalSnapshotAround (home folder CRUD)
// call it that way today because CreateInput/MoveInput carry no project id
// to resolve one from at all — a real, deliberately deferred gap (SDD
// review fix round 3; the folder-CRUD half of this same class of bug,
// tracked for Task 8/11, NOT closed here). workspaceSnapshotAround's home
// branch, which DOES have a real homeWorkspaceID to resolve one from
// (WorkspaceGitStatus.RepoIDsForHome), always passes a real set.
//
// Chat-kind rows never needed this: mergeHomeForest only ever CORRECTS an
// existing entry already scoped to the caller's own workspace (via
// ListByWorkspace), never APPENDS one — so an unrelated project's chat can
// never enter baseRows through this path in the first place. Folder-kind
// rows still leak across projects unconditionally at the bare root — the
// SAME pre-existing, disclosed gap noted throughout this task's report
// (domain.Folder has no project field at all to filter by).
//
// The returned homeIDs set is every id this walk discovered a Node row for —
// writeRow's home/repo dispatch (see writeHomeNode, plan.go). A row this walk
// never reaches (no Node row exists anywhere in its ancestor chain — the bare
// "new chat with no parent" gap, disclosed in this task's report) is simply
// absent, exactly as it already is from the read model today.
func (u *chatFolderUsecase) mergeHomeForest(
	ctx context.Context,
	baseRows []domain.Chat,
	repoMemberIDs map[string]bool,
) ([]domain.Chat, map[string]bool, error) {
	byID := make(map[string]int, len(baseRows))
	for i, row := range baseRows {
		byID[row.ID] = i
	}
	homeIDs := map[string]bool{}
	seen := map[string]bool{"": true}
	queue := []string{""}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		children, err := u.nodes.ListByParent(ctx, parent)
		if err != nil {
			return nil, nil, fmt.Errorf("agent chat folder: home forest: %w", err)
		}
		for _, n := range children {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			included, childID, err := u.mergeHomeNode(ctx, n, byID, &baseRows, repoMemberIDs)
			if err != nil {
				return nil, nil, err
			}
			if included {
				homeIDs[n.ID] = true
			}
			if childID != "" {
				queue = append(queue, childID)
			}
		}
	}
	return baseRows, homeIDs, nil
}

// mergeHomeNode handles ONE Node mergeHomeForest's BFS discovered, factored
// out to keep the walk's own loop under this package's gocyclo ceiling.
// Mutates *baseRows in place (append or correct-in-place, matching
// mergeHomeForest's own prior inline behaviour exactly). Returns whether n
// counts toward homeIDs, and the ONE child id (if any) the walk should queue
// next — folders and chats are containers (queued); repos are leaves
// (never queued: nothing files INTO a repo's own row).
func (u *chatFolderUsecase) mergeHomeNode(
	ctx context.Context,
	n domain.Node,
	byID map[string]int,
	baseRows *[]domain.Chat,
	repoMemberIDs map[string]bool,
) (included bool, childID string, err error) {
	switch n.Kind {
	case domain.NodeKindFolder:
		f, ferr := u.folders.FindByKey(ctx, n.ID)
		if ferr != nil {
			return false, "", fmt.Errorf("agent chat folder: home forest: folder %s: %w", n.ID, ferr)
		}
		if f == nil || f.RepoID != "" {
			// An orphaned Node, or (should never happen post-Task-5) a
			// repo-scoped folder row -- not this walk's to include.
			return false, "", nil
		}
		*baseRows = append(*baseRows, homeFolderView(*f, n))
		return true, n.ID, nil
	case domain.NodeKindChat:
		if i, ok := byID[n.ID]; ok {
			(*baseRows)[i].ParentID = n.ParentID
			(*baseRows)[i].Order = n.Order
		}
		return true, n.ID, nil
	case domain.NodeKindRepo:
		if repoMemberIDs != nil && !repoMemberIDs[n.ID] {
			return false, "", nil // another project's repo sharing the bare root -- see mergeHomeForest's doc
		}
		*baseRows = append(*baseRows, domain.Chat{
			ID: n.ID, Type: nodePhantomType, ParentID: n.ParentID, Order: n.Order,
		})
		return true, "", nil
	case domain.NodeKindWorkspace:
		// Not reachable at home before Task 7 -- ignore defensively.
		return false, "", nil
	}
	return false, "", nil
}

// homeSnapshotAround is workspaceSnapshotAround's home-scoped body: rows,
// already scoped to workspaceID (ListByWorkspace), merged against the home
// Node forest — see mergeHomeForest. subject is always treated as home here:
// this is only ever called once workspaceSnapshotAround has confirmed
// workspaceID itself is a project's home workspace.
//
// This is the ONE mergeHomeForest caller able to close the repo-phantom
// cross-project leak (SDD review fix round 3): unlike folder CRUD
// (globalSnapshotAround, no workspace in hand at all), a chat placement
// always has a real homeWorkspaceID to resolve a project's own repos from —
// see WorkspaceGitStatus.RepoIDsForHome.
func (u *chatFolderUsecase) homeSnapshotAround(
	ctx context.Context,
	workspaceID string,
	rows []domain.Chat,
	subject domain.Chat,
) (*treeSnapshot, error) {
	repoMemberIDs, err := u.workspaces.RepoIDsForHome(ctx, workspaceID)
	if err != nil {
		// Degrades to "do not filter" rather than failing the whole
		// placement — the same posture homeChatIDSet (project.go) takes for
		// an unresolvable home workspace. A resolution failure here means a
		// bare-root densify is no worse scoped than it was before this fix,
		// not that the operation itself should be refused.
		repoMemberIDs = nil
	}
	merged, homeIDs, err := u.mergeHomeForest(ctx, rows, repoMemberIDs)
	if err != nil {
		return nil, err
	}
	return buildHomeSnapshot(merged, subject, homeIDs, subject.ID != ""), nil
}

// buildHomeSnapshot finishes a home-aware snapshot build shared by
// globalSnapshotAround and homeSnapshotAround: it marks subject in homeIDs
// when subjectIsHome says it belongs there, and — only when mergeHomeForest's
// own walk did NOT already discover a Node row for it — marks it freshIDs
// too, so persist's write dispatch mints one instead of trying to move a row
// that does not exist yet. homeIDs is mutated and reused as the snapshot's
// own map, not copied.
func buildHomeSnapshot(
	merged []domain.Chat,
	subject domain.Chat,
	homeIDs map[string]bool,
	subjectIsHome bool,
) *treeSnapshot {
	_, discovered := homeIDs[subject.ID]
	if subjectIsHome {
		homeIDs[subject.ID] = true
	}
	snap := newTreeSnapshot(corrected(merged, subject))
	snap.homeIDs = homeIDs
	if subjectIsHome && !discovered {
		snap.freshIDs[subject.ID] = true
	}
	return snap
}

// writeHomeNode is writeRow's home-scoped body (plan.go): the same
// Reparented dispatch, against Nodes instead of Chats, mirroring project.go's
// writeNode for repos. A row snapshot.freshIDs marks has no Node row yet
// (its first-ever placement, right after a folder create or a MintChat) and
// is minted via Nodes.Create instead — determined explicitly at
// snapshot-build time (see globalSnapshotAround/workspaceSnapshotAround/
// createHomeFolder), never by probing a write's own error: the mock store
// this package's own tests run against does not surface a "no such node"
// error from SetOrder/SetPlacement, and neither, in general, should a caller
// need to parse one to know whether a row is new.
func (u *chatFolderUsecase) writeHomeNode(
	ctx context.Context,
	snapshot *treeSnapshot,
	row *domain.Chat,
) (*domain.Chat, error) {
	kind := domain.NodeKindChat
	switch row.Type {
	case domain.ChatTypeFolder:
		kind = domain.NodeKindFolder
	case domain.ChatTypeChat, domain.ChatTypeBranch, domain.ChatTypeWorkflow:
		// kind is already NodeKindChat.
	default:
		// nodePhantomType -- a repo's own Node row riding through this
		// Chat-shaped snapshot, not a member of the domain.ChatType enum.
		kind = domain.NodeKindRepo
	}
	if snapshot.freshIDs[row.ID] {
		if _, err := u.nodes.Create(ctx, row.ID, kind, row.ParentID, row.Order); err != nil {
			return nil, fmt.Errorf("agent chat folder: node create %s: %w", row.ID, err)
		}
		return row, nil
	}
	if !snapshot.plan.Reparented(row.ID) {
		if err := u.nodes.SetOrder(ctx, row.ID, row.Order); err != nil {
			return nil, fmt.Errorf("agent chat folder: node order %s: %w", row.ID, err)
		}
		return row, nil
	}
	if err := u.nodes.SetPlacement(ctx, row.ID, row.ParentID, row.Order); err != nil {
		return nil, fmt.Errorf("agent chat folder: node place %s: %w", row.ID, err)
	}
	return row, nil
}
