package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// WorktreeCreator is the narrow write port Promote needs from the worktree
// hierarchy usecase: creating the workspace a bubble promotes into, forked
// from the resolved fork parent.
//
// Declared here, by the consumer (see internal/shared/seam's own doc for why):
// this feature only ever needs ONE verb off the worktree usecase, so a field
// added to worktree.CreateChildInput for an unrelated reason cannot silently
// change what Promote passes, and this package never has to import the
// worktree usecase's wide surface to reach it.
type WorktreeCreator interface {
	CreateChildWorkspace(
		ctx context.Context,
		forkParentID string,
	) (domain.Workspace, error)
	// DiscardChildWorkspace takes a workspace CreateChildWorkspace has just
	// minted back out, for a promotion that then failed. It is the compensating
	// half of the verb above and belongs on the same port for that reason: the
	// workspace exists before the row that would own it does, and the only way
	// to reach a workspace is through that row, so a promotion abandoned in
	// between leaves a real worktree on disk that nothing can ever name again.
	DiscardChildWorkspace(
		ctx context.Context,
		workspaceID string,
	) error
}

// ErrNoForkParent is returned when chatID has no ancestor carrying a
// workspace to fork from: a bubble at the panel root, or one whose every
// ancestor is workspace-less too. There is nothing to promote it under.
var ErrNoForkParent = errors.New("agent: chat has no fork parent to promote from")

// ErrAlreadyPromoted is returned when chatID already has a workspace. Only a
// bubble (WorkspaceID == "") can be promoted — model spec §4.2's "worktree is
// never demoted" cuts the other direction too: promotion fills an empty slot
// exactly once.
var ErrAlreadyPromoted = errors.New("agent: chat already has a workspace")

// ErrNothingToPromote is returned when chatID has never had a live runner or a
// conversation: there is no "current provider" for the respawn step to resume
// as.
var ErrNothingToPromote = errors.New("agent: chat has no provider to promote with")

// Promote fills a bubble's empty workspace slot: a new worktree forked from
// the chat's resolved fork parent, with the chat's current provider respawned
// into it — see the ChatUsecase interface doc and model spec §4.2.
//
// It keeps the chat's id, its title and every turn already on it; only
// WorkspaceID changes. The respawn goes entirely through SwitchProvider — the
// same tear-down/assemble/respawn sequence an ordinary provider switch already
// performs — so this adds no new spawn machinery of its own.
func (u *Usecase) Promote(
	ctx context.Context,
	chatID string,
) (domain.Chat, error) {
	chat, err := u.GetChat(ctx, chatID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: get chat: %w", err)
	}
	if chat.WorkspaceID != "" {
		return domain.Chat{}, fmt.Errorf("promote %s: %w", chatID, ErrAlreadyPromoted)
	}
	forkParentID, ok, err := tree.ResolveForkParent(ctx, u.chats, chatID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: resolve fork parent: %w", err)
	}
	if !ok {
		return domain.Chat{}, fmt.Errorf("promote %s: %w", chatID, ErrNoForkParent)
	}
	providerID, err := u.currentProviderID(ctx, chatID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: %w", err)
	}
	ws, err := u.worktree.CreateChildWorkspace(ctx, forkParentID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: create workspace: %w", err)
	}
	if _, err := u.chats.SetWorkspace(ctx, chatID, ws.ID); err != nil {
		return domain.Chat{}, u.unpromote(ctx, chatID, ws.ID, false,
			fmt.Errorf("promote: set workspace: %w", err))
	}
	if _, err := u.runners.SwitchProvider(ctx, chatID, providerID); err != nil {
		return domain.Chat{}, u.unpromote(ctx, chatID, ws.ID, true,
			fmt.Errorf("promote: respawn: %w", err))
	}
	// Best-effort, and deliberately AFTER the respawn: the incoming CLI's own
	// handoff is assembled from the ledger as it stood BEFORE this note exists
	// (model spec §4.2 step 4 runs after step 3), so a note failure here must
	// never unwind a promotion that already succeeded.
	if err := u.conversations.NotePromotion(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "agent: promote: note promotion (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
	return u.GetChat(ctx, chatID)
}

// unpromote takes a half-finished promotion back out and hands back the failure
// that caused it, the same shape tree.Create's discardFolder and CreateChat's
// discard already use for a create that failed after minting something.
//
// It rolls the WHOLE promotion back rather than leaving a chat sitting on a
// worktree with no CLI in it: Promote refuses a chat that already has a
// workspace (ErrAlreadyPromoted), so a half-promoted row cannot be retried —
// the user would be stuck with it. Undoing both steps leaves the retry they
// will make the same call they just made.
//
// slotFilled says whether the row was ever pointed at the workspace, and the
// ORDER it forces is what makes this safe: the workspace is discarded only once
// nothing owns it. A clear that fails therefore keeps the worktree, because
// deleting it under a row that still names it would turn a failed promotion
// into a chat anchored to a directory that is gone.
//
// Every step is best-effort and none of them replaces the cause. What failed is
// the promotion, and that is what the caller is told.
func (u *Usecase) unpromote(
	ctx context.Context,
	chatID string,
	workspaceID string,
	slotFilled bool,
	cause error,
) error {
	if slotFilled {
		if _, err := u.chats.SetWorkspace(ctx, chatID, ""); err != nil {
			slog.WarnContext(ctx, "agent: promote: roll back the workspace slot",
				"chat_id", chatID, "workspace_id", workspaceID, "err", err)
			return cause
		}
	}
	if err := u.worktree.DiscardChildWorkspace(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "agent: promote: discard the workspace nothing came to own",
			"chat_id", chatID, "workspace_id", workspaceID, "err", err)
	}
	return cause
}

// currentProviderID answers "the same provider" step 3 respawns as: whichever
// provider is live on the chat right now, or — for a dormant bubble — the
// provider its last conversation was with. Mirrors ResumeChat's own live/last
// resolution (internal/runner/lifecycle.go) rather than inventing a second one.
func (u *Usecase) currentProviderID(
	ctx context.Context,
	chatID string,
) (string, error) {
	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ProviderID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("current provider: live runner: %w", err)
	}
	convs, err := u.runners.ConversationsForChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("current provider: conversations: %w", err)
	}
	if len(convs) == 0 {
		return "", ErrNothingToPromote
	}
	return convs[len(convs)-1].ProviderID, nil
}
