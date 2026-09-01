package tree

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The boot backfill: the one-shot reconciliation that gives every workspace
// made before the atomic create — a workspace and the chat that owns it minted
// in one breath — the owning row it never got.
//
// It mints, places and retypes, and nothing else. Every workspace it processes
// already has its worktree on disk (or is a home row, which has none by
// design), so the worktree-provisioning half of the atomic create has no
// counterpart here.

// BackfillOwningChats gives every workspace the owning chat row it is owed:
// minting one where there is none, and ADOPTING the row a workspace already has
// where that workspace has since become something else — a worktree that was
// locked after its row was minted owns a branch row from then on (see adopt).
//
// It is safe to run on every boot: a workspace whose row is already right is
// passed over, so a second run over the same daemon writes nothing.
func (u *chatFolderUsecase) BackfillOwningChats(
	ctx context.Context,
) error {
	workspaces, err := u.roster.List(ctx)
	if err != nil {
		return fmt.Errorf("agent chat folder: backfill owning chats: workspaces: %w", err)
	}
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent chat folder: backfill owning chats: chats: %w", err)
	}
	spoken, err := u.spokenIn(ctx, adoptCandidates(workspaces, rows))
	if err != nil {
		return err
	}
	plan := planBackfill(workspaces, rows, spoken, uuid.NewString)
	return errors.Join(u.adoptAll(ctx, plan.Adopt), u.mintAll(ctx, plan.Mint))
}

// spokenIn asks, of each row the plan might adopt, whether anything was ever
// said in it. It is asked HERE rather than inside the plan so the plan stays a
// pure function of what it was handed, and only of rows that are actually
// candidates — a boot does not read the turn record of every chat in the
// daemon.
func (u *chatFolderUsecase) spokenIn(
	ctx context.Context,
	candidates []string,
) (map[string]bool, error) {
	spoken := make(map[string]bool, len(candidates))
	for _, id := range candidates {
		said, err := u.agent.HasTurns(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("agent chat folder: backfill owning chats: turns of %s: %w", id, err)
		}
		spoken[id] = said
	}
	return spoken, nil
}

// adoptCandidates names the rows the plan may ask about: the owning row of a
// branch-destined workspace that is not already a branch row. Every other
// workspace either needs nothing or needs a row minted, and neither asks.
func adoptCandidates(
	workspaces []domain.Workspace,
	rows []domain.Chat,
) []string {
	live := liveWorkspaces(workspaces)
	owners := owningRows(rows)
	ids := make([]string, 0)
	for _, ws := range rootsFirst(live) {
		owner := owners[ws.ID]
		if owningChatType(ws) == domain.ChatTypeBranch &&
			owner.ID != "" &&
			owner.Type != domain.ChatTypeBranch {
			ids = append(ids, owner.ID)
		}
	}
	return ids
}

// adoptAll rewrites the kind of each row the plan decided to adopt. Like
// mintAll it carries on past a failure: the rows are independent, and a row it
// could not adopt this boot is planned again on the next one.
func (u *chatFolderUsecase) adoptAll(
	ctx context.Context,
	ids []string,
) error {
	failures := make([]error, 0)
	for _, id := range ids {
		if _, err := u.chats.SetType(ctx, id, domain.ChatTypeBranch); err != nil {
			failures = append(failures,
				fmt.Errorf("agent chat folder: backfill: adopt %s as a branch row: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

// mintAll writes the planned rows in the order they were planned — roots
// first, so a row's parent always exists by the time the row names it.
//
// A failure does not abandon the workspaces behind it: each is an independent
// row, and the next boot re-plans whatever this one could not write. The one
// thing it refuses to do is write a row under a parent that failed, which would
// file it under an id nothing answers to.
func (u *chatFolderUsecase) mintAll(
	ctx context.Context,
	steps []backfillStep,
) error {
	missing := map[string]bool{}
	failures := make([]error, 0)
	for _, step := range steps {
		if missing[step.ParentID] {
			missing[step.ChatID] = true
			continue
		}
		if err := u.mintOwningChat(ctx, step); err != nil {
			missing[step.ChatID] = true
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// mintOwningChat creates one owed row and places it. A placement that fails
// after the mint leaves the row at the panel root rather than nowhere — the
// workspace owns a chat either way, which is what every read of it asks.
func (u *chatFolderUsecase) mintOwningChat(
	ctx context.Context,
	step backfillStep,
) error {
	if _, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          step.ChatID,
		WorkspaceID: step.WorkspaceID,
		Type:        step.Type,
		Now:         time.Now(),
	}); err != nil {
		return fmt.Errorf("agent chat folder: backfill %s: mint: %w", step.WorkspaceID, err)
	}
	if _, err := u.chats.SetPlacement(ctx, step.ChatID, step.ParentID, step.Order); err != nil {
		return fmt.Errorf("agent chat folder: backfill %s: place: %w", step.WorkspaceID, err)
	}
	return nil
}

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

// backfillPlan is everything one boot decided to do: rows to mint, and rows
// already standing whose KIND is now wrong and must be rewritten in place.
//
// Adopt carries chat ids and no order, because a retype changes nothing a later
// step reads — the row keeps its id, its placement and its conversation, so
// nothing depends on when it happens relative to a mint.
type backfillPlan struct {
	Mint  []backfillStep
	Adopt []string
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
	plan := backfillPlan{Mint: make([]backfillStep, 0, len(live)), Adopt: make([]string, 0)}
	for _, ws := range rootsFirst(live) {
		kind := owningChatType(ws)
		owner := owners[ws.ID]
		if adopt(owner, kind, spoken) {
			plan.Adopt = append(plan.Adopt, owner.ID)
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
