package tree

import (
	"context"
	"errors"
	"fmt"
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
	plan, err := u.planOwningChats(ctx, domain.Workspace{})
	if err != nil {
		return err
	}
	return u.apply(ctx, plan)
}

// EnsureOwningChat is BackfillOwningChats for ONE workspace, for the moment
// that workspace changes character while the daemon is running rather than
// between boots: a branch locked by the user, or a provider poll reporting one
// protected, is branch-destined from that instant, and every reader downstream
// expects the branch row to be there already.
//
// It takes the SAME decision by the SAME code — one plan, narrowed to the
// workspace asked about (see backfillPlan.forWorkspace). Nothing about
// adopt-versus-mint is restated here, because a second copy of that judgement
// is exactly how the boot pass and the live path would drift apart.
//
// ws is the workspace AS THE CALLER HOLDS IT, not an id, and that is
// load-bearing: see correctedWorkspace.
func (u *chatFolderUsecase) EnsureOwningChat(
	ctx context.Context,
	ws domain.Workspace,
) error {
	plan, err := u.planOwningChats(ctx, ws)
	if err != nil {
		return err
	}
	return u.apply(ctx, plan.forWorkspace(ws.ID))
}

// planOwningChats reads the census and the forest once and decides what every
// workspace is owed, writing nothing. subject is the one workspace the caller
// already holds a fresher answer about, or the zero value at boot, when nobody
// does.
func (u *chatFolderUsecase) planOwningChats(
	ctx context.Context,
	subject domain.Workspace,
) (backfillPlan, error) {
	listed, err := u.roster.List(ctx)
	if err != nil {
		return backfillPlan{}, fmt.Errorf("agent chat folder: owning chats: workspaces: %w", err)
	}
	workspaces := correctedWorkspace(listed, subject)
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return backfillPlan{}, fmt.Errorf("agent chat folder: owning chats: chats: %w", err)
	}
	spoken, err := u.spokenIn(ctx, adoptCandidates(workspaces, rows))
	if err != nil {
		return backfillPlan{}, err
	}
	return planBackfill(workspaces, rows, spoken, uuid.NewString), nil
}

// correctedWorkspace replaces the PROJECTED row for subject with the one the
// caller already holds — the same correction this package makes for a chat it
// has just written (see corrected, plan.go), for the same reason.
//
// It is what makes the runtime path work at all. A lock is written to the
// workspace aggregate on the ASYNC send path, so the census read a microsecond
// later can still be serving the status the workspace had BEFORE the lock. A
// reconcile planning on that stale row decides the workspace is not
// branch-destined and does nothing whatsoever — the reconcile silently no-ops
// and the branch row appears only at the next boot, which is the exact failure
// this trigger exists to prevent.
//
// A subject the census does not carry is appended rather than skipped, matching
// corrected: the workspace demonstrably exists, since the caller is holding it.
func correctedWorkspace(
	rows []domain.Workspace,
	subject domain.Workspace,
) []domain.Workspace {
	if subject.ID == "" {
		return rows
	}
	for i := range rows {
		if rows[i].ID == subject.ID {
			rows[i] = subject
			return rows
		}
	}
	return append(rows, subject)
}

// apply writes a plan: adopts first, then mints. The order does not matter to
// the rows themselves — a retype changes no id anything else reads — but it
// keeps a workspace that is BOTH adopting its own row and parenting a child's
// new one from being read mid-change.
func (u *chatFolderUsecase) apply(
	ctx context.Context,
	plan backfillPlan,
) error {
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
	steps []adoptStep,
) error {
	failures := make([]error, 0)
	for _, step := range steps {
		if _, err := u.chats.SetType(ctx, step.ChatID, domain.ChatTypeBranch); err != nil {
			failures = append(failures,
				fmt.Errorf("agent chat folder: adopt %s as %s's branch row: %w",
					step.ChatID, step.WorkspaceID, err))
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
