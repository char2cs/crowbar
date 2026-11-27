package repositories

import (
	"context"
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/purge"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type ReactorDrain struct {
	Ctx    context.Context
	Gate   *drain.Gate
	Cancel context.CancelFunc
}

func (c *Container) QuiesceReactors(
	ctx context.Context,
) {
	gate := c.drainGate
	gate.Hold()
	defer gate.Release()
	for {
		gate.WaitRunning(ctx)
		c.WaitQuiescent()
		gate.WaitRunning(ctx)
		if gate.Parked() == 0 || ctx.Err() != nil {
			return
		}
		gate.Release()
		gate.Hold()
	}
}

func (c *Container) wireCallbacks(
	ctx context.Context,
	crowbarHome string,
	onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
) error {
	c.drainGate = drain.New()
	//nolint:gosec // G118: drainCancel is deliberately retained on the container and invoked later by the app layer's graceful shutdown via Drain().Cancel, not leaked.
	c.drainCtx, c.drainCancel = context.WithCancel(ctx)

	registrar, ok := c.Workspace.(workspace.DeleteReactorRegistrar)
	if !ok {
		return fmt.Errorf("workspace repository does not support delete-reactor registration")
	}
	if err := registrar.RegisterDeleteReactor(ctx, c.forgetDependents, purge.WorktreeRemover(crowbarHome, c.Workspace.List), c.drainGate, onGiveUp); err != nil {
		return fmt.Errorf("delete reactor: %w", err)
	}
	return nil
}

func (c *Container) Drain() ReactorDrain {
	return ReactorDrain{Ctx: c.drainCtx, Gate: c.drainGate, Cancel: c.drainCancel}
}

func (c *Container) forgetReviewThreads(
	ctx context.Context,
	wsID string,
) error {
	threads, err := c.ReviewThread.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("repositories: delete cascade: list review threads for %q: %w", wsID, err)
	}
	for _, t := range threads {
		if err := c.ReviewThread.DeleteThread(ctx, t.ID); err != nil {
			return fmt.Errorf("repositories: delete cascade: forget review thread %q: %w", t.ID, err)
		}
	}
	return nil
}

func (c *Container) forgetDependents(
	ctx context.Context,
	wsID string,
) error {
	if err := c.forgetReviewThreads(ctx, wsID); err != nil {
		return err
	}
	if err := c.purgeWorkspaceChats(ctx, wsID); err != nil {
		return err
	}
	return c.forgetNode(ctx, wsID)
}

func (c *Container) purgeWorkspaceChats(
	ctx context.Context,
	wsID string,
) error {
	chats, err := c.AgentChat.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("repositories: delete cascade: list agent chats for %q: %w", wsID, err)
	}
	if len(chats) > 0 && c.PurgeChat == nil {
		return fmt.Errorf("repositories: delete cascade: no chat purge wired")
	}
	for _, chat := range chats {
		if err := c.PurgeChat(ctx, chat.ID); err != nil && !alreadyGone(err) {
			return fmt.Errorf("repositories: delete cascade: purge chat %q: %w", chat.ID, err)
		}
		if err := c.forgetNode(ctx, chat.ID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Container) forgetNode(
	ctx context.Context,
	id string,
) error {
	if err := c.Placement.Forget(ctx, id); err != nil && !alreadyGone(err) {
		return fmt.Errorf("repositories: delete cascade: forget node %q: %w", id, err)
	}
	return nil
}

func alreadyGone(err error) bool {
	return errors.Is(err, problem.ErrNotFound) ||
		errors.Is(err, agentchat.ErrNotFound) ||
		errors.Is(err, asynxModels.ErrValidation)
}
