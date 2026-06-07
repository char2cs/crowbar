package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProjectActivityRollup is the best-effort project lastActivity roll-up surface.
// The chat package keeps its own narrow copy so it never imports the project
// usecase; the container passes the concrete project usecase, which satisfies it.
type ProjectActivityRollup interface {
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)
}

// LifecycleRepo is the chat repository surface the usecase needs.
type LifecycleRepo interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		title string,
		now time.Time,
	) (domain.Chat, error)
	Fork(
		ctx context.Context,
		id string,
		wsID string,
		parentID string,
		title string,
		now time.Time,
	) (domain.Chat, error)
	Rename(
		ctx context.Context,
		id string,
		title string,
	) (domain.Chat, error)
	Delete(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

// WorkspaceRepo is the workspace surface the chat usecase needs to roll up
// activity onto the parent workspace and its project.
type WorkspaceRepo interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	TouchActivity(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// Usecase is the chat lifecycle surface: create/fork/rename/cascade-delete,
// plus the flat per-workspace read that backs the chat sidebar.
type Usecase interface {
	// ListChatsByWorkspace returns the flat list of chats for a workspace.
	ListChatsByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)

	// CreateChat mints an id, issues the create command, and rolls up activity on
	// the workspace and its project.
	CreateChat(
		ctx context.Context,
		wsID string,
		title string,
		now time.Time,
	) (domain.Chat, error)

	// ForkChat loads the parent chat for its workspace and title, then forks.
	ForkChat(
		ctx context.Context,
		parentID string,
		now time.Time,
	) (domain.Chat, error)

	// RenameChat renames a chat.
	RenameChat(
		ctx context.Context,
		id string,
		title string,
	) (domain.Chat, error)

	// DeleteChat deletes a chat and cascades to its descendants. Delete is
	// idempotent so replay is safe.
	DeleteChat(
		ctx context.Context,
		id string,
		now time.Time,
	) error
}

type chatUsecase struct {
	chats      LifecycleRepo
	workspaces WorkspaceRepo
	rollup     ProjectActivityRollup
	now        func() time.Time
}

// New builds a Usecase from the chat repo, workspace repo, and project roll-up
// usecase. now supplies the timestamp for activity roll-ups on the lifecycle
// methods that do not receive one (rename).
func New(
	chats LifecycleRepo,
	workspaces WorkspaceRepo,
	rollup ProjectActivityRollup,
	now func() time.Time,
) Usecase {
	return &chatUsecase{
		chats:      chats,
		workspaces: workspaces,
		rollup:     rollup,
		now:        now,
	}
}

// ListChatsByWorkspace returns the flat list of chats for a workspace.
func (u *chatUsecase) ListChatsByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	chats, err := u.chats.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("chat: list by workspace: %w", err)
	}
	return chats, nil
}

// CreateChat mints an id, issues the create command, and rolls up activity on
// the workspace and its project.
func (u *chatUsecase) CreateChat(
	ctx context.Context,
	wsID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	created, err := u.chats.Create(ctx, uuid.NewString(), wsID, title, now)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: create: %w", err)
	}
	u.rollupActivity(ctx, wsID, now)
	return created, nil
}

// ForkChat loads the parent chat for its workspace and title, then forks.
func (u *chatUsecase) ForkChat(
	ctx context.Context,
	parentID string,
	now time.Time,
) (domain.Chat, error) {
	parent, err := u.chats.Get(ctx, parentID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: fork: load parent: %w", err)
	}
	forked, err := u.chats.Fork(ctx, uuid.NewString(), parent.WsID, parentID, parent.Title, now)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: fork: %w", err)
	}
	u.rollupActivity(ctx, parent.WsID, now)
	return forked, nil
}

// RenameChat renames a chat and rolls up activity on its workspace and project,
// symmetric with create/fork (renaming a chat is workspace activity).
func (u *chatUsecase) RenameChat(
	ctx context.Context,
	id string,
	title string,
) (domain.Chat, error) {
	renamed, err := u.chats.Rename(ctx, id, title)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: rename: %w", err)
	}
	u.rollupActivity(ctx, renamed.WsID, u.now())
	return renamed, nil
}

// DeleteChat deletes a chat and cascades to its descendants. Delete is
// idempotent so replay is safe. It rolls up workspace/project activity,
// symmetric with create/fork.
func (u *chatUsecase) DeleteChat(
	ctx context.Context,
	id string,
	now time.Time,
) error {
	root, err := u.chats.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("chat: delete: load: %w", err)
	}
	siblings, err := u.chats.ListByWorkspace(ctx, root.WsID)
	if err != nil {
		return fmt.Errorf("chat: delete: list: %w", err)
	}
	if err := u.deleteSubtree(ctx, id, siblings, now, map[string]bool{}); err != nil {
		return err
	}
	u.rollupActivity(ctx, root.WsID, now)
	return nil
}

func (u *chatUsecase) deleteSubtree(
	ctx context.Context,
	id string,
	all []domain.Chat,
	now time.Time,
	visited map[string]bool,
) error {
	if visited[id] {
		return nil
	}
	visited[id] = true
	for _, child := range all {
		if child.ParentID != id {
			continue
		}
		if err := u.deleteSubtree(ctx, child.ID, all, now, visited); err != nil {
			return err
		}
	}
	if _, err := u.chats.Delete(ctx, id, now); err != nil {
		return fmt.Errorf("chat: delete: %w", err)
	}
	return nil
}

func (u *chatUsecase) rollupActivity(
	ctx context.Context,
	wsID string,
	now time.Time,
) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return
	}
	_, _ = u.workspaces.TouchActivity(ctx, wsID, now)
	u.rollup.TouchProjectActivity(ctx, ws.RepoID, now)
}
