package repositories

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	ReviewThread reviewthread.ReviewThread
	hub          hub.WebSocketHub
	git          wsusecase.MergeConflictChecker
	// inflight counts the background mutations currently running per workspace
	// id (00 §4 fail-fast/good-path-async). It backs the derived Working overlay:
	// the API layer brackets each async op with BeginWork/EndWork, and every
	// serving path (live broadcast, snapshot, REST reads) overlays IsWorking so
	// the client spinner tracks real daemon activity.
	mu       sync.Mutex
	inflight map[string]int
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. The workspace aggregate is backed by the singleton axWorkspace (one
// instance per type, routing every id by shard hash) built by the app layer; its
// read model lives in state/store/workspace.db and its id↔path index in view.db.
// The chat and reviewthread aggregates keep their global event stores and read
// models in the global view DB (converted in Tasks 12/13).
func New(
	ctx context.Context,
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axChat asynx.Asynx[domain.Chat],
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	git wsusecase.MergeConflictChecker,
) (*Container, error) {
	c := &Container{hub: h, git: git, inflight: map[string]int{}}
	pathsStore, err := wspaths.NewWorkspacePaths(adapters.GlobalView())
	if err != nil {
		return nil, fmt.Errorf("repositories: workspace paths: %w", err)
	}
	ws, err := workspace.New(axWorkspace, adapters.WorkspaceES(), adapters.WorkspaceView(), pathsStore)
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	// Register the hub (WS fan-out) projection on the singleton axWorkspace,
	// injecting the container-owned enrichment: every event-driven frame goes
	// through the SAME enrichFrame + hub.BroadcastWorkspace as the BeginWork/EndWork
	// rebroadcasts, so the FE spinner + merge badges survive the store/hub split
	// with zero regression (spec §3.5 hub-frame enrichment). The save-only store
	// projection is registered inside workspace.New; the two derive independently
	// from evt.Aggregate and cannot drift (decision 5).
	if err := workspace.RegisterHubProjection(axWorkspace, c.enrichFrame, c.hub.BroadcastWorkspace); err != nil {
		return nil, fmt.Errorf("repositories: workspace hub projection: %w", err)
	}
	db := adapters.GlobalView()
	ch, err := chat.New(ctx, axChat, adapters.ChatES(), db, func(domain.Chat) {})
	if err != nil {
		return nil, err
	}
	// reviewthread now owns its own central per-type read model at
	// state/store/review_thread.db (Task 12), no longer the shared view.db: pass
	// ReviewThreadView() as the read-model DB while keeping ReviewThreadES() for the
	// lazy AggregateLister Replay (§3.7).
	rt, err := reviewthread.New(axReviewThread, adapters.ReviewThreadES(), adapters.ReviewThreadView(), func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Chat = ch
	c.ReviewThread = rt
	return c, nil
}

// enrichFrame builds the WS frame for ws: it attaches the two derived overlays
// that are NOT part of the event-sourced aggregate — the Working/inflight spinner
// (bracketed by BeginWork/EndWork, 00 §4) and the merge-eligibility overlay
// (CanMergeLocally/ParentBranch, resolved off the hot path from the row's
// repo-scoped siblings, spec §10) — and returns the wire DTO. It is the SINGLE
// enrichment both the hub projection (RegisterHubProjection) and the
// BeginWork/EndWork rebroadcasts converge on, so the emitted frame is identical
// regardless of trigger (spec §3.5 hub-frame enrichment).
func (c *Container) enrichFrame(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspaceDTO {
	ws.Working = c.IsWorking(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	return dto.WorkspaceDTOFrom(ws, elig)
}

// broadcastWorkspace enriches ws and pushes it to the hub. It backs the
// BeginWork/EndWork rebroadcasts (which fire on the 202 ack, not on an event) and
// routes through the SAME enrichFrame as the hub projection so both agree.
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	c.hub.BroadcastWorkspace(c.enrichFrame(ctx, ws))
}

// BeginWork marks the start of a background mutation on the workspace and
// immediately re-broadcasts its row with Working=true, so the client spinner
// starts the moment the 202 is written — not when the op's first event lands.
// Blank ids (a create that has not produced an entity yet) are ignored.
// Concurrent ops on the same workspace nest: the overlay stays true until the
// matching EndWork of the LAST one.
func (c *Container) BeginWork(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	c.mu.Lock()
	c.inflight[wsID]++
	c.mu.Unlock()
	c.rebroadcast(ctx, wsID)
}

// EndWork marks the end of a background mutation on the workspace and
// re-broadcasts its row so the final frame always carries Working=false (and
// whatever LastError the op recorded). Unbalanced calls never underflow, and a
// row deleted by the op itself just skips the re-broadcast (its tombstone
// already rode the event stream).
func (c *Container) EndWork(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	c.mu.Lock()
	if n := c.inflight[wsID]; n <= 1 {
		delete(c.inflight, wsID)
	} else {
		c.inflight[wsID] = n - 1
	}
	c.mu.Unlock()
	c.rebroadcast(ctx, wsID)
}

// IsWorking reports whether the workspace has a background mutation in flight.
func (c *Container) IsWorking(
	wsID string,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inflight[wsID] > 0
}

// rebroadcast pushes the workspace's current row to the hub so an overlay
// transition (Begin/EndWork) is visible without waiting for the op's next
// event. Best-effort: a missing row (already deleted) skips silently.
func (c *Container) rebroadcast(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		return
	}
	c.broadcastWorkspace(ctx, ws)
}

// eligibilityFor resolves the merge-eligibility overlay (incl. the predicted
// merge-conflict flag) for ws by reading its siblings and delegating to the
// shared wsusecase.ResolveMergeEligibility — the SAME resolver the snapshot read
// path uses, so the live broadcast and the snapshot always agree. The sibling
// read is best effort — a failed List degrades to no eligibility rather than
// dropping the broadcast.
func (c *Container) eligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
) wsusecase.MergeEligibility {
	if ws.ParentID == "" {
		return wsusecase.MergeEligibility{}
	}
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return wsusecase.MergeEligibility{}
	}
	return wsusecase.ResolveMergeEligibility(ctx, ws, rows, c.git)
}

// ListWorkspaces returns every workspace row with the derived Working overlay
// applied, so a snapshot-on-subscribe taken mid-mutation agrees with the live
// broadcast frames. It backs the Workspaces snapshot-on-subscribe.
func (c *Container) ListWorkspaces(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.IsWorking(rows[i].ID)
	}
	return rows, nil
}
