package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The checks every move passes before anything is written. Folder ops
// (repo-wide, no workspace of their own) and chat ops (workspace-scoped) carry
// different container rules, so each gets its own pair below.

// checkFolderMove refuses a move onto a container that does not exist, lies
// inside the moved row's own subtree, would give it a parent outside its own
// repo scope, or would cross it into a DIFFERENT context within that same
// repo (the finer half of the golden rule — see checkFolderContextMove's own
// doc). The context check runs only for a move, never a create: a brand new
// folder has no prior position to have crossed away from — it simply takes
// on whatever context its chosen parent already implies.
func (u *chatFolderUsecase) checkFolderMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	folderRepoID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkFolderContainer(ctx, snapshot, folderRepoID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	if err := u.checkFolderContextMove(ctx, snapshot, id, destination); err != nil {
		return err
	}
	return nil
}

// checkWorkspaceMove refuses a LOCKED BRANCH's own placement move (2026-09-09,
// PlaceWorkspace) onto a container that does not exist, lies inside its own
// subtree, or would leave its own repo scope — the identical golden rule
// checkFolderContainer already enforces for a folder, reused as-is: repoID is
// the branch's own RepoOf answer, playing the part folderRepoID plays for a
// folder.
//
// It deliberately does NOT run checkFolderContextMove, unlike checkFolderMove.
// That finer check exists to stop a CHILD row silently crossing from one
// anchor to another — but the branch itself IS one of the four anchor tiers
// (Project -> Repo -> Locked branch -> Parent unlocked branch, see
// nearestWorkspaceAnchor's own doc): nearestWorkspaceAnchor(id) resolves to
// id itself before it ever walks up, since id's own Node.Kind already reads
// NodeKindWorkspace. Reusing checkFolderMove wholesale would therefore compare
// the branch's own id against the destination's (almost always different)
// anchor and refuse EVERY legal move with ErrCrossContext — caught before
// shipping by tracing what nearestWorkspaceAnchor(workspaceID) actually
// answers, not by a live failure. The repo-scope check below is the only
// invariant the branch's own placement owes: see planTreeRowDrop
// (drop-actions.ts) for the frontend contract this mirrors — a locked branch
// only ever moves within its OWN repo, to its bare root or one of that
// repo's own folders (or another branch's own row, nested organisation, not a
// fork-lineage change — see checkFolderContainer's row-type acceptance).
func (u *chatFolderUsecase) checkWorkspaceMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	repoID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkFolderContainer(ctx, snapshot, repoID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkFolderContextMove is the golden rule's fine grain: "context", in
// order, is Project -> Repo -> Locked branch -> Parent unlocked (branch) —
// four different anchors a folder can sit under, not just two (home vs a
// repo). A folder may move freely among its own siblings and descendants
// WITHIN whatever anchor it already sits under, but never jump to a
// different one — a different branch's own subtree, the bare repo root
// versus any branch, or project-home versus a repo — even though
// checkFolderContainer's repo-level check alone would allow all of those
// (same repo, or both "").
//
// The anchor itself is never stored: it is answered by walking the live Node
// position (2026-09-08 sidebar-placement-unification Task 8 — see parentOf)
// until a row with a real WorkspaceID turns up (a locked or unlocked
// branch's own owning row) — reaching the root without finding one means
// the anchor is the bare repo root (or project home, already distinguished
// by folderRepoID's own "" convention). Comparing this walk's answer for the
// folder's CURRENT position against the same walk for its PROPOSED one is
// the whole check; nothing here is written or persisted.
//
// Walking Node.ParentID rather than Chat.ParentID matters the moment a row's
// placement is Node-backed: Chat.ParentID freezes at creation for such a
// row (writeRow's dispatch, plan.go), so a walk that trusted it would answer
// against a row's ORIGINAL container forever, not wherever it has since been
// dragged.
func (u *chatFolderUsecase) checkFolderContextMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
	destination string,
) error {
	from, err := u.nearestWorkspaceAnchor(ctx, snapshot, id)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s: %w", id, err)
	}
	to, err := u.nearestWorkspaceAnchor(ctx, snapshot, destination)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, err)
	}
	if from != to {
		return fmt.Errorf(
			"agent chat folder: move %s under %s crosses context: %w",
			id, destination, ErrCrossContext,
		)
	}
	return nil
}

// nearestWorkspaceAnchor walks id's ParentID chain (id's OWN row included)
// until a row that genuinely IS a workspace turns up, and answers that
// workspace's id. "" is a real answer, not "not found": it means the walk
// reached the panel root without ever crossing one — the bare repo root, or
// project home.
//
// "Is a workspace" is answered directly off the Node forest: id's own
// Node.Kind == NodeKindWorkspace, full stop (2026-09-08
// sidebar-placement-unification Task 9 — every workspace mints a
// Node{Kind:workspace} row, keyed by its own id, unconditionally at
// creation, so this is never ambiguous). Deliberately NOT "carries a
// non-empty WorkspaceID" — a plain chat carries one too (domain.Chat's own
// doc: "WorkspaceID always resolves to a repo", populated on every row
// regardless of Type), but that is the chat's GROUND, the workspace whose
// API scope it happens to live under, not a context boundary. Stopping at
// "carries a WorkspaceID" made a folder sitting at the bare root (whose own
// walk correctly falls through folders, which never carry WorkspaceID,
// straight to "") compare unequal against a PLAIN CHAT SIBLING at that
// identical root — the chat's walk stopped one step early, on itself, and
// reported its own ground workspace as if it were an anchor. Two rows at the
// exact same position in the exact same context reported different anchors
// purely because one kind (chat) happens to store a WorkspaceID pointer the
// other kind (folder) never does — caught live as a folder that could
// reorder past a chat sibling but could never be filed INTO one, refused
// with "crosses context" for two rows that had never left it.
func (u *chatFolderUsecase) nearestWorkspaceAnchor(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (string, error) {
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return "", nil
		}
		seen[id] = true
		n, nErr := u.nodes.GetNode(ctx, id)
		if nErr == nil && n.Kind == domain.NodeKindWorkspace {
			return id, nil
		}
		if nErr == nil {
			id = n.ParentID
			continue
		}
		// A row Node has never touched (a proxy owning chat minted before its
		// worktree existed, still placed through placeOwningRow directly
		// rather than through Nodes) falls back to its own frozen Chat.ParentID
		// — exactly as live as it always was, since nothing but placeOwningRow
		// ever writes it.
		row, err := u.resolveRow(ctx, snapshot, id)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", id, err)
		}
		id = row.ParentID
	}
	return "", nil
}

// resolveRow answers id's Chat-shaped view for a validation walk: the
// snapshot's own copy when it has one (already Node-corrected by
// mergeForest, for whichever rows that walk discovered — see its own doc),
// falling back in turn to a keyed Chats.Get, a keyed Folders+Nodes read — a
// folder mergeForest's own BFS happened not to reach (its "seed every known
// chat id" walk is thorough but not exhaustive against every conceivable
// ancestor chain; see mergeForest's own doc) is still a legitimate container
// the golden rule must be able to resolve, exactly as a chat the global list
// did not carry already is (TestCreate_AcceptsAChatTheGlobalListDidNotCarry)
// — and, only once BOTH of those come back not-found, a keyed Nodes read for
// a workspace's own Node{Kind:workspace} row (see workspaceAnchorView): the
// placement id MintOwningChat/importPlacement resolve a new row's ParentID
// to (owning_chat.go, 2026-09-08 sidebar-placement-unification Task 9) names
// no Chat or Folder aggregate at all, only that Node row.
func (u *chatFolderUsecase) resolveRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.Chat, error) {
	if row := snapshot.row(id); row != nil {
		return row, nil
	}
	got, err := u.chats.Get(ctx, id)
	if err == nil {
		return &got, nil
	}
	if f, ferr := u.folders.FindByKey(ctx, id); ferr == nil && f != nil {
		n, nerr := u.nodes.GetNode(ctx, id)
		if nerr != nil {
			n = domain.Node{}
		}
		row := homeFolderView(*f, n)
		return &row, nil
	}
	if n, nerr := u.nodes.GetNode(ctx, id); nerr == nil && n.Kind == domain.NodeKindWorkspace {
		row := workspaceAnchorView(id, n)
		return &row, nil
	}
	return nil, err // the ORIGINAL Chats.Get failure -- neither a folder nor a workspace's own Node row answers to id either.
}

// checkFolderContainer validates a folder's parent id: "" is the panel root
// (always legal — nothing to inherit a scope FROM, and folderRepoID is
// already this folder's own fixed scope, set once at creation and never
// rewritten by a move), and anything else must be a row that exists AND
// share folderRepoID.
//
// The scoping golden rule: a folder may only be filed under a parent whose
// own scope is the SAME one this folder already has — "" (project-home) with
// "", a repo id only with that same repo id. This is what makes ListInRepo's
// filter (tree.go) actually TRUE rather than merely asserted: nothing can
// reach a repo's tree, or home's, that does not already carry that repo's id.
// Caught live as a folder or chat left "on top of" the wrong repo after being
// dragged there.
func (u *chatFolderUsecase) checkFolderContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	folderRepoID string,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	row, err := u.resolveRow(ctx, snapshot, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	parentRepoID, err := u.repoScopeOf(ctx, *row)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if parentRepoID != nil && *parentRepoID != folderRepoID {
		return fmt.Errorf(
			"agent chat folder: parent %s belongs to repo %q, not %q: %w",
			parentID, *parentRepoID, folderRepoID, ErrCrossRepo,
		)
	}
	return nil
}

// repoScopeOf answers a row's own repo scope, or nil when the row carries
// none to check against (a plain chat bubble with no workspace of its own —
// nothing to conflict with, same posture checkParentKind already takes for a
// chat parent in the identical situation).
//
// A FOLDER answers its own stored RepoID directly — no lookup. A row that
// OWNS a workspace (a BRANCH, or a CHAT forked with its own worktree)
// resolves it via WorkspaceGitStatus.RepoOf, the one case that needs the
// workspace layer at all.
func (u *chatFolderUsecase) repoScopeOf(ctx context.Context, row domain.Chat) (*string, error) {
	if row.Type == domain.ChatTypeFolder {
		id := row.RepoID
		return &id, nil
	}
	if row.WorkspaceID == "" {
		return nil, nil
	}
	id, err := u.workspaces.RepoOf(ctx, row.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// checkChatMove refuses a chat move onto a container that does not exist,
// belongs to another workspace, or lies inside the moved chat's own subtree.
//
// ownWorktree is threaded straight from placeChat: true only for its
// own-worktree-creation caller, createOwnWorktreeChat (see placeChat's doc
// comment) — never for an ordinary move, which always passes false.
func (u *chatFolderUsecase) checkChatMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	id string,
	destination string,
	ownWorktree bool,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkChatContainer(ctx, snapshot, workspaceID, destination, ownWorktree); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkChatContainer validates a chat's parent id: "" is the panel root, a
// FOLDER or a BRANCH is accepted unconditionally (neither carries a workspace
// to conflict with — a branch row is a process boundary, not a workspace one),
// and a CHAT must belong to workspaceID — unless ownWorktree names an
// own-worktree creation forking off a chat that already owns a worktree of its
// own, which is legal for the same reason forking off a BRANCH row is (see
// checkParentKind). A row that resolves to a DIFFERENT workspace is reported
// as a cross-workspace edge rather than as a missing row, because the two are
// fixed in different ways.
func (u *chatFolderUsecase) checkChatContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if parentID == "" {
		return nil
	}
	row, err := u.resolveRow(ctx, snapshot, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	return checkParentKind(*row, workspaceID, parentID, ownWorktree)
}

// checkParentKind is the second half of checkChatContainer, split out because
// both the snapshot-membership path and the keyed-lookup fallback answer the
// same question about the row once they have it.
//
// The keyed read heals the chat read model for the one id it was asked about;
// the workspace list only heals a model that is entirely empty — so the
// authoritative answer here can name a row the snapshot's list did not carry.
// Refusing it would reject a drop onto a chat the user can see.
//
// ownWorktree is true only for an own-worktree CREATION (never a move, and
// never an ordinary thread): forking a new workspace off a row that already
// carries one is legal regardless of that row's kind, exactly as it already is
// off a BRANCH row — the new chat has no workspace yet to conflict with
// anything, so a worktree-owning CHAT parent is accepted the same way a
// FOLDER or BRANCH parent is. An ordinary thread still enforces the
// same-workspace rule against a CHAT parent: it inherits that parent's cwd, so
// one claiming a different workspace than its parent is meaningless.
func checkParentKind(
	row domain.Chat,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	// workspaceAnchorType (a locked branch's own row, 2026-09-09) is accepted
	// unconditionally for the identical reason ChatTypeBranch already is: a
	// process boundary, not a workspace one, and a locked branch is exactly
	// the "1:N chats" container spec §2.4 describes. Relying only on the
	// WorkspaceID == workspaceID fallback below would happen to work too
	// (workspaceAnchorView sets it to the anchor's own id), but this makes
	// the acceptance explicit rather than coincidental.
	if row.Type == domain.ChatTypeFolder || row.Type == domain.ChatTypeBranch || row.Type == workspaceAnchorType {
		return nil
	}
	if ownWorktree && row.WorkspaceID != "" {
		return nil
	}
	if row.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, row.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}
