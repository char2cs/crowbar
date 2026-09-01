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
// It mints and places, and nothing else. Every workspace it processes already
// has its worktree on disk (or is a home row, which has none by design), so
// the worktree-provisioning half of the atomic create has no counterpart here.

// BackfillOwningChats mints the owning chat row of every workspace that has
// none. It is safe to run on every boot: a workspace that already owns a row is
// passed over, so a second run over the same daemon mints nothing.
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
	return u.mintAll(ctx, planBackfill(workspaces, rows, uuid.NewString))
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

// planBackfill decides the whole backfill from one census of the workspaces and
// one read of the forest, minting nothing.
//
// Deciding first and writing second is what makes the root-first walk possible:
// a row's ParentID is the chat id its FORK PARENT is about to be minted under,
// which is known here and could not be read back out of a projection that
// trails every write.
func planBackfill(
	workspaces []domain.Workspace,
	rows []domain.Chat,
	newID func() string,
) []backfillStep {
	live := liveWorkspaces(workspaces)
	owning := owningChatIDs(rows)
	slots := siblingCounts(rows)
	steps := make([]backfillStep, 0, len(live))
	for _, ws := range rootsFirst(live) {
		if _, owned := owning[ws.ID]; owned {
			continue
		}
		parentID := owning[forkParentOf(ws, live)]
		id := newID()
		steps = append(steps, backfillStep{
			ChatID:      id,
			WorkspaceID: ws.ID,
			Type:        owningChatType(ws),
			ParentID:    parentID,
			Order:       slots[parentID],
		})
		slots[parentID]++
		owning[ws.ID] = id
	}
	return steps
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

// owningChatIDs maps each workspace to the chat row that ALREADY owns it — the
// gate the whole backfill turns on, and the answer a child's placement resolves
// against when its fork parent was backfilled on an earlier boot.
//
// Rows carrying no workspace (folders, and chats still waiting for one) own
// nothing and are left out, so a lookup for the empty workspace id answers ""
// — the panel root — rather than some unrelated bubble's id. Where a workspace
// somehow carries several rows the EARLIEST wins, matching the creation-order
// tiebreak the rest of the tree sorts on.
func owningChatIDs(
	rows []domain.Chat,
) map[string]string {
	owners := make(map[string]domain.Chat, len(rows))
	for _, row := range rows {
		if row.WorkspaceID == "" {
			continue
		}
		if held, ok := owners[row.WorkspaceID]; ok && earlier(held, row) {
			continue
		}
		owners[row.WorkspaceID] = row
	}
	ids := make(map[string]string, len(owners))
	for wsID, row := range owners {
		ids[wsID] = row.ID
	}
	return ids
}

func earlier(
	a domain.Chat,
	b domain.Chat,
) bool {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c < 0
	}
	return a.ID < b.ID
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
