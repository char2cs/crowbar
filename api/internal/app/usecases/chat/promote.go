package chat

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// WorktreeCreator is the narrow write port this feature needs from the worktree
// hierarchy usecase, declared by the consumer so the worktree usecase's wide surface
// never leaks in.
type WorktreeCreator interface {
	// CreateChildWorkspace forks a fresh workspace off forkParentID; a blank branch
	// gets a server-generated name.
	CreateChildWorkspace(
		ctx context.Context,
		forkParentID string,
		branch string,
	) (domain.Workspace, error)
	// CreateImportedWorkspace is CreateChildWorkspace's import counterpart: the
	// workspace for a branch that already exists, named outright by spec.
	CreateImportedWorkspace(
		ctx context.Context,
		spec ImportSpec,
	) (domain.Workspace, error)
	// DiscardChildWorkspace removes a just-minted workspace whose create then failed:
	// a workspace is reachable only through its owning row, so an abandoned one
	// would be a worktree on disk nothing can name again.
	DiscardChildWorkspace(
		ctx context.Context,
		workspaceID string,
	) error
}

// ErrNoForkParent is returned when chatID has no ancestor carrying a
// workspace to fork from: a bubble at the panel root, or one whose every
// ancestor is workspace-less too. There is nothing to promote it under.
var ErrNoForkParent = problem.New(problem.Conflict, "agent: chat has no fork parent to promote from")

// ErrAlreadyPromoted is returned when chatID already has a workspace: only a bubble
// (WorkspaceID == "") can be promoted, and promotion fills an empty slot exactly once.
var ErrAlreadyPromoted = problem.New(problem.Conflict, "agent: chat already has a workspace")

// ErrNothingToPromote is returned when chatID has never had a live runner or a
// conversation: there is no "current provider" for the respawn step to resume
// as.
var ErrNothingToPromote = problem.New(problem.Conflict, "agent: chat has no provider to promote with")

// Promote fills a bubble's empty workspace slot with a worktree forked from its fork
// parent and respawns the current provider into it via SwitchProvider. Only
// WorkspaceID changes; id, title and turns are kept.
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
	forkParentID, ok, err := tree.ResolveForkParent(ctx, u.chats, u.folders, u.nodes, u.repoRoots, chatID)
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
	ws, err := u.worktree.CreateChildWorkspace(ctx, forkParentID, "")
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: create workspace: %w", err)
	}
	if _, err := u.chats.SetWorkspace(ctx, chatID, ws.ID); err != nil {
		return domain.Chat{}, u.unpromote(ctx, chatID, ws.ID, false,
			fmt.Errorf("promote: set workspace: %w", err))
	}
	if _, err := u.runners.SwitchProvider(ctx, chatID, providerID, ""); err != nil {
		return domain.Chat{}, u.unpromote(ctx, chatID, ws.ID, true,
			fmt.Errorf("promote: respawn: %w", err))
	}
	// Best-effort and AFTER the respawn, whose handoff predates this note: a note
	// failure must never unwind a promotion that already succeeded.
	if err := u.conversations.NotePromotion(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "agent: promote: note promotion (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
	return u.GetChat(ctx, chatID)
}

// unpromote rolls a half-finished promotion WHOLLY back, best-effort, and returns
// cause: a half-promoted row could never be retried. The workspace is discarded only
// once no row names it, so a failed clear keeps it rather than orphan the chat.
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

// currentProviderID is the provider Promote respawns as: the live one, else the
// chat's own durable vendor.
func (u *Usecase) currentProviderID(
	ctx context.Context,
	chatID string,
) (string, error) {
	if live, err := u.runners.LiveRunnerForChat(ctx, chatID); err == nil {
		return live.ProviderID, nil
	}
	chat, err := u.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("current provider: chat: %w", err)
	}
	if chat.ProviderID == "" {
		return "", ErrNothingToPromote
	}
	return chat.ProviderID, nil
}
