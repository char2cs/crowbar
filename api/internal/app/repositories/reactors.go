package repositories

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"context"
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/purge"
)

// ReactorDrain is the shutdown handle for every post-commit reactor: the app layer
// closes the gate (Cancel) so reactors start no new work, then waits on it before
// the adapter closes the DBs. Ctx is the parent the gate derives from.
type ReactorDrain struct {
	Ctx    context.Context
	Gate   *drain.Gate
	Cancel context.CancelFunc
}

// QuiesceReactors is WaitQuiescent for a mutation whose effect lands OUTSIDE the
// aggregate (a delete cascade, a pull's child resync): every projection drained
// AND every reactor the drained events admitted run, round after round until a
// round admits none. WaitPublish waits only for what was queued when called, so
// the door is held (drain.Gate.Hold): a reactor admitted mid-round parks and runs
// between rounds. ctx is the caller's escape hatch, not a synchronisation device.
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

// wireCallbacks registers the cross-aggregate reactions (spec §3.6): the workspace
// delete reactor with the forgetDependents cascade and the worktree remover, all
// joined to the drain gate Drain() hands the app layer's ordered shutdown.
func (c *Container) wireCallbacks(
	ctx context.Context,
	crowbarHome string,
) error {
	c.drainGate = drain.New()
	//nolint:gosec // G118: drainCancel is deliberately retained on the container and invoked later by the app layer's graceful shutdown via Drain().Cancel, not leaked.
	c.drainCtx, c.drainCancel = context.WithCancel(ctx)

	// Registered through the repository's seam, which builds the one Purger the
	// reactor and the boot sweep share; a new dependent joins forgetDependents.
	registrar, ok := c.Workspace.(workspace.DeleteReactorRegistrar)
	if !ok {
		return fmt.Errorf("workspace repository does not support delete-reactor registration")
	}
	if err := registrar.RegisterDeleteReactor(ctx, c.forgetDependents, purge.WorktreeRemover(crowbarHome, c.Workspace.List), c.drainGate); err != nil {
		return fmt.Errorf("delete reactor: %w", err)
	}
	return nil
}

// Drain is the reactor drain handle for the app layer's ordered shutdown.
func (c *Container) Drain() ReactorDrain {
	return ReactorDrain{Ctx: c.drainCtx, Gate: c.drainGate, Cancel: c.drainCancel}
}

// forgetReviewThreads forgets every review thread anchored to the deleted
// workspace; each Forget's marker drops the thread's read-model row.
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

// forgetDependents removes what a deleted workspace owns besides its root on disk
// (invariant D5). Each step tolerates what an interrupted run already removed, so
// a re-drive converges; any other failure leaves the tombstone for the next one.
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

// purgeWorkspaceChats hard-deletes every chat anchored to the workspace, with
// its Node row.
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

// forgetNode drops id's Node row, tolerating one that is already gone.
func (c *Container) forgetNode(
	ctx context.Context,
	id string,
) error {
	if err := c.Node.Forget(ctx, id); err != nil && !alreadyGone(err) {
		return fmt.Errorf("repositories: delete cascade: forget node %q: %w", id, err)
	}
	return nil
}

// alreadyGone reports an error that means the thing being removed no longer
// exists: a not-found, or asynx refusing to Forget an aggregate that is already
// Forgotten. For a delete cascade both are success.
func alreadyGone(err error) bool {
	return errors.Is(err, problem.ErrNotFound) ||
		errors.Is(err, agentchat.ErrNotFound) ||
		errors.Is(err, asynxModels.ErrValidation)
}
