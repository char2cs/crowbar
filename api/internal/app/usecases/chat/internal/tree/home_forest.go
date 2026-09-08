package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The Node-backed read side, home AND repo scope alike (2026-09-08
// sidebar-placement-unification Task 5 built this for home; Task 8 widens it
// to repo-scoped folders/chats too — the two scopes converge onto ONE walk,
// differing only in whether repo phantoms participate and which project's
// repos are allowed to): rendering a Folder+Node pair as the Chat-shaped view
// the rest of this package plans and broadcasts against, and mergeForest —
// the Node-native forest walk that replaced project.go's cross-aggregate
// merge for home, and now also replaces the repo-scoped folder CRUD's own
// former Chat-only walk.

// homeFolderView renders a domain.Folder+domain.Node pair as the same
// Chat-shaped row every other verb in this package plans and broadcasts —
// the wire contract does not change, only where a folder's identity/position
// are sourced from. Despite the name (kept from Task 5 to minimise churn),
// this renders BOTH home (RepoID=="") and repo-scoped (RepoID!="") folders
// alike since Task 8 — f.RepoID rides straight through untouched. n's zero
// value is a legitimate answer: a folder whose Node row has not been minted
// yet (this call IS the mint) sits at the panel root with Order 0, exactly
// like a freshly-minted repo or chat.
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
// through Chats, only Nodes (see mergeForest, writeHomeNode). It is never
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

// correctHomePlacement overrides a Node-backed chat row's ParentID/Order with
// its live Node row — home-scoped since Task 5, repo-scoped too since Task 8
// (any chat whose workspace is neither the bubble scope nor unresolvable).
// Chat.ParentID/.Order are write-once-at-creation for such a row from this
// task onward (position authority moves to Node) — full cleanup of the now-
// dead fields is Task 11's job, but a caller reading row.ParentID for
// ANYTHING placement-related (origin container, lineage, cwd) needs the live
// answer NOW, not the frozen one. A row still not yet minted onto Node (its
// very first placement) keeps its Chat-native (zero-value) placement, which
// is what a brand new bubble already carries.
func (u *chatFolderUsecase) correctHomePlacement(
	ctx context.Context,
	row domain.Chat,
) (domain.Chat, error) {
	if row.WorkspaceID == "" {
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

// mergeForest walks the Node forest breadth-first from the panel root AND
// from every chat id baseRows already names (see the seeding loop below),
// folding it into baseRows: a real chat row already in baseRows has its
// ParentID/Order corrected to match its live Node row; a Folder (home OR
// repo-scoped alike, since Task 8 — a folder's own RepoID rides straight
// through, unchecked here; checkFolderContainer is what enforces the golden
// rule against it) is appended as a homeFolderView; a repo's own Node row is
// appended as a nodePhantomType placeholder purely so it participates in the
// SAME densify/sibling-count pass as its chat/folder siblings sharing
// project home's own bare root — this is the direct replacement for
// project.go's now-deleted cross-aggregate merge: every home-scope sibling
// now comes from ONE Node.ListByParent walk.
//
// includeRepoPhantoms is false for a repo-scoped (non-home) chat placement's
// own merge (Task 8): a repo is never a legitimate sibling INSIDE a repo's
// own internal tree (only at project home, where a repo's header row sits
// among home's own chats/folders) — including it there would let a
// repo-internal densify renumber, and WRITE, an unrelated repo's own Node
// row. true for the home-level merge (globalSnapshotAround, homeSnapshotAround)
// exactly as it always has been.
//
// repoMemberIDs further scopes which repo phantoms are included, when
// includeRepoPhantoms is true — the SAME reason project.go's
// placeRepoAmongHomeSiblings needs repoIDSet/homeChatIDSet (SDD review
// Critical 2): domain.Node carries no project id of its own, so a repo
// sharing the bare root ("") belongs to SOME project, not necessarily the
// caller's, and appending it unscoped would let a densify in one project
// renumber — and WRITE, via writeHomeNode's Nodes.SetOrder/.SetPlacement —
// another project's repo Node row. nil means "do not filter":
// globalSnapshot/globalSnapshotAround (folder CRUD, home AND repo scope
// alike) call it that way today because CreateInput/MoveInput carry no
// project id to resolve one from at all — a real, deliberately deferred gap
// (SDD review fix round 3; the folder-CRUD half of this same class of bug,
// tracked for Task 8/11, NOT closed here). homeSnapshotAround's home branch,
// which DOES have a real homeWorkspaceID to resolve one from
// (WorkspaceGitStatus.RepoIDsForHome), always passes a real set.
//
// The BFS is seeded not only with "" but with every id baseRows already
// names (2026-09-08 sidebar-placement-unification Task 8): before this,
// EVERY repo-scoped folder was itself a Chat row, so a folder filed under an
// ordinary chat or a locked branch's own owning row (which never gets a Node
// of its own — see writeHomeNode's dispatch and MintOwningChat/
// placeOwningRow, owning_chat.go, untouched by this task) was ALREADY present
// in baseRows (ListChats' own flat read) with the right ParentID, no walk
// needed. Once repo-scoped folders leave Chat entirely, a folder nested under
// such a row is invisible to a walk that only ever discovers Node children of
// OTHER NODES — nothing queues "branch-1" as a parent to probe, because
// "branch-1" itself never gets a Node row under this task's scope. Seeding
// the queue with every known chat id closes that gap: Node.ListByParent is
// asked about every chat id baseRows already has, discovering whatever was
// filed under it regardless of whether that chat itself is Node-backed.
//
// The returned homeIDs set is every id this walk discovered a Node row for —
// writeRow's home/repo dispatch (see writeHomeNode, plan.go). A row this walk
// never reaches (no Node row exists anywhere in its ancestor chain — the bare
// "new chat with no parent" gap, disclosed in Task 5's report, unchanged by
// Task 8) is simply absent, exactly as it already is from the read model
// today.
func (u *chatFolderUsecase) mergeForest(
	ctx context.Context,
	baseRows []domain.Chat,
	includeRepoPhantoms bool,
	repoMemberIDs map[string]bool,
) ([]domain.Chat, map[string]bool, error) {
	byID := make(map[string]int, len(baseRows))
	for i, row := range baseRows {
		byID[row.ID] = i
	}
	homeIDs := map[string]bool{}
	nodeSeen := map[string]bool{}
	queried := map[string]bool{}
	queue := make([]string, 0, len(baseRows)+1)
	queue = append(queue, "")
	for _, row := range baseRows {
		queue = append(queue, row.ID)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if queried[parent] {
			continue
		}
		queried[parent] = true
		children, err := u.nodes.ListByParent(ctx, parent)
		if err != nil {
			return nil, nil, fmt.Errorf("agent chat folder: node forest: %w", err)
		}
		for _, n := range children {
			if nodeSeen[n.ID] {
				continue
			}
			nodeSeen[n.ID] = true
			included, childID, err := u.mergeHomeNode(ctx, n, byID, &baseRows, includeRepoPhantoms, repoMemberIDs)
			if err != nil {
				return nil, nil, err
			}
			if included {
				homeIDs[n.ID] = true
			}
			if childID != "" && !queried[childID] {
				queue = append(queue, childID)
			}
		}
	}
	return baseRows, homeIDs, nil
}

// mergeHomeNode handles ONE Node mergeForest's BFS discovered, factored out
// to keep the walk's own loop under this package's gocyclo ceiling. Mutates
// *baseRows in place (append or correct-in-place, matching mergeForest's own
// prior inline behaviour exactly). Returns whether n counts toward homeIDs,
// and the ONE child id (if any) the walk should queue next — folders and
// chats are containers (queued); repos are leaves (never queued: nothing
// files INTO a repo's own row from this walk's own discovery — a repo-scoped
// folder/chat sitting "under" a repo is filed at the bare root, "", or under
// one of its branches' own owning chat rows, both already covered by the
// queue's own seeding).
func (u *chatFolderUsecase) mergeHomeNode(
	ctx context.Context,
	n domain.Node,
	byID map[string]int,
	baseRows *[]domain.Chat,
	includeRepoPhantoms bool,
	repoMemberIDs map[string]bool,
) (included bool, childID string, err error) {
	switch n.Kind {
	case domain.NodeKindFolder:
		f, ferr := u.folders.FindByKey(ctx, n.ID)
		if ferr != nil {
			return false, "", fmt.Errorf("agent chat folder: node forest: folder %s: %w", n.ID, ferr)
		}
		if f == nil {
			// An orphaned Node -- not this walk's to include.
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
		if !includeRepoPhantoms {
			// A repo is never a legitimate sibling INSIDE a repo's own
			// internal tree -- see mergeForest's own doc.
			return false, "", nil
		}
		if repoMemberIDs != nil && !repoMemberIDs[n.ID] {
			return false, "", nil // another project's repo sharing the bare root -- see mergeForest's doc
		}
		*baseRows = append(*baseRows, domain.Chat{
			ID: n.ID, Type: nodePhantomType, ParentID: n.ParentID, Order: n.Order,
		})
		return true, "", nil
	case domain.NodeKindWorkspace:
		// Not reachable at home before Task 7 -- ignore defensively. A
		// repo-scoped branch's own owning row is not reachable here either
		// (untouched by Task 8, still governed entirely by owning_rows.go/
		// placeOwningRow, never given a Node of its own).
		return false, "", nil
	}
	return false, "", nil
}

// homeSnapshotAround is workspaceSnapshotAround's Node-backed body: rows,
// already scoped to workspaceID (ListByWorkspace), merged against the Node
// forest — see mergeForest. subject is always treated as Node-backed here:
// this is only ever called once workspaceSnapshotAround has confirmed
// workspaceID resolves (home or repo alike, since Task 8 — see the home
// parameter).
//
// home selects the SAME repo-phantom-inclusion split mergeForest's own doc
// describes: true for a project's home workspace (a repo's header row IS a
// legitimate sibling there), false for a repo-scoped workspace (a repo is
// never a sibling of its OWN internal chats/folders). Only the home branch
// resolves repoMemberIDs at all — the SDD review fix round 3 leak this
// closes (mergeForest's own repo-phantom cross-project scoping) has no
// repo-scoped counterpart, since a repo-scoped call never includes repo
// phantoms in the first place.
func (u *chatFolderUsecase) homeSnapshotAround(
	ctx context.Context,
	workspaceID string,
	home bool,
	rows []domain.Chat,
	subject domain.Chat,
) (*treeSnapshot, error) {
	var repoMemberIDs map[string]bool
	if home {
		repoMemberIDs = u.repoMemberIDsForHome(ctx, workspaceID)
	}
	merged, homeIDs, err := u.mergeForest(ctx, rows, home, repoMemberIDs)
	if err != nil {
		return nil, err
	}
	return buildHomeSnapshot(merged, subject, homeIDs, subject.ID != ""), nil
}

// repoMemberIDsForHome resolves the SAME project's repo id set
// RepoIDsForHome answers, degrading to "do not filter" (nil) on a
// resolution failure rather than failing the whole placement — the same
// posture homeChatIDSet (project.go) takes for an unresolvable home
// workspace. A resolution failure here means a bare-root densify is no
// worse scoped than it was before this fix, not that the operation itself
// should be refused. Split out of homeSnapshotAround only to keep that
// function's own nesting flat.
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

// buildHomeSnapshot finishes a Node-aware snapshot build shared by
// globalSnapshotAround and homeSnapshotAround: it marks subject in homeIDs
// when subjectIsHome says it belongs there, and — only when mergeForest's
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

// writeHomeNode is writeRow's Node-backed body (plan.go): the same
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
