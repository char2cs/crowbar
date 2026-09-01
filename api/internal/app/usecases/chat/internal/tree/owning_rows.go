package tree

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// What each workspace is OWED: the whole adopt-or-mint judgement, taken as a
// pure function of one workspace census and one read of the forest.
//
// It is separated from the passes that read and write (backfill.go) because it
// is the half worth reasoning about on its own — nothing here touches a store,
// so every rule below can be read, and tested, without a daemon around it. The
// boot pass and the runtime trigger both come here, which is what keeps them
// from drifting apart.

// backfillStep is one workspace's owed row, decided in full before anything is
// written: the id it will be minted under, the kind of row it is, and where it
// hangs.
type backfillStep struct {
	ChatID      string
	WorkspaceID string
	Type        domain.ChatType
	ParentID    string
	Order       int
}

// backfillPlan is everything one pass decided to do: rows to mint, and rows
// already standing whose KIND is now wrong and must be rewritten in place.
//
// Adopt carries no order, because a retype changes nothing a later step reads —
// the row keeps its id, its placement and its conversation, so nothing depends
// on when it happens relative to a mint.
type backfillPlan struct {
	Mint  []backfillStep
	Adopt []adoptStep
}

// adoptStep is one standing row whose kind is now wrong. It names the workspace
// as well as the row so a plan can be narrowed to a single workspace — see
// forWorkspace.
type adoptStep struct {
	ChatID      string
	WorkspaceID string
}

// forWorkspace narrows a whole-daemon plan to the one workspace a runtime
// trigger asked about, PLUS every row that workspace's own placement depends on.
//
// The plan is computed across every workspace either way, and it has to be: a
// row's ParentID is its fork parent's own owning row, so the decision cannot be
// taken for one workspace in isolation. What narrows is only what gets WRITTEN
// — a user locking one branch does not silently reconcile every other workspace
// in the daemon on its way through.
//
// The ancestors are the exception, and they are not a courtesy. A repo imported
// since the last boot has a whole chain nothing has backfilled, so the target's
// ParentID can name a row this same plan is about to mint for its fork parent.
// Writing the target without that parent files it under an id nothing answers
// to — and the damage is PERMANENT: SetPlacement validates no parent, so the
// phantom is written happily, and alreadyOwned then reports the workspace
// satisfied on every later boot, so no backfill ever repairs it. mintAll's own
// guard does not catch this either; it skips a child whose parent step FAILED,
// and a step that was never in the slice fails nothing.
//
// Only MINT steps are pulled in. An ancestor that merely needs retyping already
// has its row, so the target's ParentID names something real whether or not that
// retype happens, and the boot pass will take it.
func (p backfillPlan) forWorkspace(
	workspaceID string,
) backfillPlan {
	needed := p.mintsBehind(workspaceID)
	narrowed := backfillPlan{Mint: make([]backfillStep, 0, len(needed)), Adopt: make([]adoptStep, 0, 1)}
	// Ranged over the ORIGINAL slice, so the survivors keep the root-first order
	// planBackfill put them in — which is what lets mintAll rely on a parent
	// being written before the row that names it.
	for _, step := range p.Mint {
		if needed[step.ChatID] {
			narrowed.Mint = append(narrowed.Mint, step)
		}
	}
	for _, step := range p.Adopt {
		if step.WorkspaceID == workspaceID {
			narrowed.Adopt = append(narrowed.Adopt, step)
		}
	}
	return narrowed
}

// mintsBehind is the set of planned rows workspaceID's own row cannot be written
// without: its own, and the unbroken chain of planned ancestors it hangs off.
//
// The walk stops as soon as a ParentID names something the plan is NOT minting,
// which is the ordinary case — that id is a row already standing.
func (p backfillPlan) mintsBehind(
	workspaceID string,
) map[string]bool {
	planned := make(map[string]backfillStep, len(p.Mint))
	for _, step := range p.Mint {
		planned[step.ChatID] = step
	}
	needed := make(map[string]bool, len(p.Mint))
	for _, step := range p.Mint {
		if step.WorkspaceID != workspaceID {
			continue
		}
		for at := step; !needed[at.ChatID]; {
			needed[at.ChatID] = true
			parent, ok := planned[at.ParentID]
			if !ok {
				break
			}
			at = parent
		}
	}
	return needed
}

// planBackfill decides the whole backfill from one census of the workspaces and
// one read of the forest, writing nothing.
//
// Deciding first and writing second is what makes the root-first walk possible:
// a row's ParentID is the chat id its FORK PARENT is about to be minted under,
// which is known here and could not be read back out of a projection that
// trails every write.
//
// spoken answers, for a row that might be adopted, whether anything was ever
// said in it. See adopt.
func planBackfill(
	workspaces []domain.Workspace,
	rows []domain.Chat,
	spoken map[string]bool,
	newID func() string,
) backfillPlan {
	live := liveWorkspaces(workspaces)
	owners := owningRows(rows)
	slots := siblingCounts(rows)
	plan := backfillPlan{Mint: make([]backfillStep, 0, len(live)), Adopt: make([]adoptStep, 0)}
	for _, ws := range rootsFirst(live) {
		kind := owningChatType(ws)
		owner := owners[ws.ID]
		if adopt(owner, kind, spoken) {
			plan.Adopt = append(plan.Adopt, adoptStep{ChatID: owner.ID, WorkspaceID: ws.ID})
			owner.Type = domain.ChatTypeBranch
			owners[ws.ID] = owner
			continue
		}
		if alreadyOwned(owner, kind) {
			continue
		}
		parentID := owners[forkParentOf(ws, live)].ID
		id := newID()
		plan.Mint = append(plan.Mint, backfillStep{
			ChatID:      id,
			WorkspaceID: ws.ID,
			Type:        kind,
			ParentID:    parentID,
			Order:       slots[parentID],
		})
		slots[parentID]++
		owners[ws.ID] = domain.Chat{ID: id, Type: kind, WorkspaceID: ws.ID}
	}
	return plan
}

// adopt reports whether a workspace's existing owning row should simply BECOME
// the branch row rather than have one minted beside it.
//
// It exists because what a workspace IS can change after its row is minted: a
// worktree backfilled as an ordinary chat row is locked a week later, by the
// user or by the provider reporting its branch protected, and is branch-destined
// from then on. Minting a second row there would leave two rows claiming one
// workspace — the exact invariant this backfill exists to establish — so the row
// it already has is rewritten in place, keeping its id, its placement and its
// history.
//
// The guard is that NOTHING WAS EVER SAID in the row, and it is the whole reason
// this is safe. An owning row the backfill minted and a conversation somebody
// started inside the same workspace are the same shape — chat-typed, carrying
// the workspace id — and no field tells them apart. Turns do. A branch row does
// not open as a conversation, so adopting one that holds somebody's words would
// hide them; a row with words in it therefore gets a branch row minted beside it
// instead, exactly as a workspace with several candidate rows does.
func adopt(
	owner domain.Chat,
	kind domain.ChatType,
	spoken map[string]bool,
) bool {
	if kind != domain.ChatTypeBranch || owner.ID == "" {
		return false
	}
	if owner.Type == domain.ChatTypeBranch {
		return false
	}
	return !spoken[owner.ID]
}

// alreadyOwned is the idempotency gate, and it asks a DIFFERENT question of the
// two row kinds.
//
// For a workspace owed a BRANCH row — locked, repo home, project home — the
// question is whether a BRANCH row owns it, because type is structural there. An
// ordinary chat left over from before this backfill carries the workspace id
// exactly as an owning row does (every chat started inside a workspace does),
// but it is not drawn as a branch row and was not even an acceptable parent
// until checkParentKind was widened — so "some chat mentions this workspace"
// would leave the branch permanently without the row the sidebar addresses it
// by. Nothing in this daemon writes a branch row except this backfill, so on an
// install that never had one the narrower question is the same question.
//
// For every other workspace it is the original question — any chat at all —
// because that is already what resolveOwningChat answers with, so a second row
// would only recreate the ambiguity this gate exists to prevent.
func alreadyOwned(
	owner domain.Chat,
	kind domain.ChatType,
) bool {
	if kind == domain.ChatTypeBranch {
		return owner.Type == domain.ChatTypeBranch
	}
	return owner.ID != ""
}

// liveWorkspaces indexes every workspace the backfill may mint for. A
// tombstoned row — Status "deleted", the boot sweep's own quarry — is not one:
// it is on its way out, and a chat minted onto it would outlive it.
func liveWorkspaces(
	workspaces []domain.Workspace,
) map[string]domain.Workspace {
	live := make(map[string]domain.Workspace, len(workspaces))
	for _, ws := range workspaces {
		if ws.Status != domain.WorkspaceStatusDeleted {
			live[ws.ID] = ws
		}
	}
	return live
}

// owningChatType answers which KIND of row a workspace owns. A locked branch, a
// repo home and a project home are BRANCH rows; every other worktree owns an
// ordinary chat row. Status is the whole of the lock question — it already
// folds the user's override and the provider's protected flag into one answer
// — so there is nothing else to consult.
func owningChatType(
	ws domain.Workspace,
) domain.ChatType {
	if ws.Status == domain.WorkspaceStatusLocked ||
		ws.IsDefault ||
		ws.Kind == domain.WorkspaceKindHome {
		return domain.ChatTypeBranch
	}
	return domain.ChatTypeChat
}

// forkParentOf resolves the workspace whose owning row this one's hangs off, or
// "" for one that hangs at the panel root. A repo or project home is a root by
// what it IS, and so is a row whose recorded fork parent is no longer there.
func forkParentOf(
	ws domain.Workspace,
	live map[string]domain.Workspace,
) string {
	if ws.IsDefault || ws.Kind == domain.WorkspaceKindHome {
		return ""
	}
	if _, ok := live[ws.ParentID]; !ok {
		return ""
	}
	return ws.ParentID
}

// owningRows maps each workspace to the row that ALREADY owns it — what the
// gate is asked about, and what a child's placement resolves against when its
// fork parent was backfilled on an earlier boot.
//
// Rows carrying no workspace (folders, and chats still waiting for one) own
// nothing and are left out, so a lookup for the empty workspace id answers the
// zero row — the panel root — rather than some unrelated bubble's id.
func owningRows(
	rows []domain.Chat,
) map[string]domain.Chat {
	owners := make(map[string]domain.Chat, len(rows))
	for _, row := range rows {
		if row.WorkspaceID == "" {
			continue
		}
		if held, ok := owners[row.WorkspaceID]; ok && preferred(held, row) {
			continue
		}
		owners[row.WorkspaceID] = row
	}
	return owners
}

// ResolveOwningChat picks the row that owns a workspace from its candidate
// chat rows — typically everything Chats.ListByWorkspace(workspaceID)
// returns — applying the SAME branch-preference tiebreak BackfillOwningChats
// enforces (preferred, below). It is exported so a caller outside this
// package that already holds a workspace's chat rows can answer "which one
// owns it" without re-deriving that rule: the workspace wire DTO's
// owningChatId field is the first such caller.
func ResolveOwningChat(
	rows []domain.Chat,
) (domain.Chat, bool) {
	if len(rows) == 0 {
		return domain.Chat{}, false
	}
	owner := rows[0]
	for _, row := range rows[1:] {
		if !preferred(owner, row) {
			owner = row
		}
	}
	return owner, true
}

// preferred reports whether held keeps the workspace against challenger.
//
// A BRANCH row always wins. Once one exists it IS the workspace's row — what
// the sidebar draws and what a child has to hang off — and a locked branch that
// was chatted in before this backfill has ordinary rows OLDER than it, so a
// plain creation-order tiebreak would quietly make that branch's children
// threads of somebody's old conversation. Between rows of the same standing the
// earlier one wins, matching the tiebreak the rest of the tree sorts on.
func preferred(
	held domain.Chat,
	challenger domain.Chat,
) bool {
	if (held.Type == domain.ChatTypeBranch) != (challenger.Type == domain.ChatTypeBranch) {
		return held.Type == domain.ChatTypeBranch
	}
	if c := held.CreatedAt.Compare(challenger.CreatedAt); c != 0 {
		return c < 0
	}
	return held.ID < challenger.ID
}

// siblingCounts is how many rows already sit in each sibling space, which is
// also the first free index in it — the answer the plan's NextSlot gives a
// newly created folder, computed here without a plan because the backfill
// writes rows that are not in the forest yet.
func siblingCounts(
	rows []domain.Chat,
) map[string]int {
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.ParentID]++
	}
	return counts
}

// rootsFirst orders the census so every workspace comes after its fork parent —
// breadth-first from the roots, same-parent siblings in creation order.
// That order is the contract, not a preference: a row names the row minted for
// its parent, so the parent's has to be written first.
//
// A workspace no root reaches — a fork-parent cycle, which no write path can
// produce but a damaged log could — is appended rather than dropped, so "every
// live workspace ends up owning a row" holds even then.
func rootsFirst(
	live map[string]domain.Workspace,
) []domain.Workspace {
	children := make(map[string][]domain.Workspace, len(live))
	for _, ws := range live {
		parent := forkParentOf(ws, live)
		children[parent] = append(children[parent], ws)
	}
	for parent := range children {
		slices.SortFunc(children[parent], byCreation)
	}
	ordered := make([]domain.Workspace, 0, len(live))
	seen := make(map[string]bool, len(live))
	queue := slices.Clone(children[""])
	for at := 0; at < len(queue); at++ {
		ws := queue[at]
		if seen[ws.ID] {
			continue
		}
		seen[ws.ID] = true
		ordered = append(ordered, ws)
		queue = append(queue, children[ws.ID]...)
	}
	return append(ordered, unreached(live, seen)...)
}

func unreached(
	live map[string]domain.Workspace,
	seen map[string]bool,
) []domain.Workspace {
	rest := make([]domain.Workspace, 0)
	for id, ws := range live {
		if !seen[id] {
			rest = append(rest, ws)
		}
	}
	slices.SortFunc(rest, byCreation)
	return rest
}

func byCreation(
	a domain.Workspace,
	b domain.Workspace,
) int {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}
