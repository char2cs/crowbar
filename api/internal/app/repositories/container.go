package repositories

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
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
// the hub. The workspace aggregate is per-entity event-sourced: its Asynx
// instances and view DBs are resolved lazily from the adapter container by ID,
// using the injected asynxFactory (passed by the app layer to avoid an import
// cycle on newAsynx). The chat and reviewthread aggregates keep their global
// event stores and read models in the global view DB.
func New(
	ctx context.Context,
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axChat asynx.Asynx[domain.Chat],
	axReviewThread asynx.Asynx[domain.ReviewThread],
	asynxFactory workspace.AsynxFactory,
	git wsusecase.MergeConflictChecker,
) (*Container, error) {
	c := &Container{hub: h, git: git, inflight: map[string]int{}}
	ws, err := workspace.New(adapters, func(ctx context.Context, w domain.Workspace) {
		c.broadcastWorkspace(ctx, w)
	}, asynxFactory)
	if err != nil {
		return nil, err
	}
	db := adapters.GlobalView()
	ch, err := chat.New(ctx, axChat, adapters.ChatES(), db, func(domain.Chat) {})
	if err != nil {
		return nil, err
	}
	rt, err := reviewthread.New(ctx, axReviewThread, adapters.ReviewThreadES(), db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	c.Chat = ch
	c.ReviewThread = rt
	return c, nil
}

// broadcastWorkspace converts a workspace row to its wire DTO and pushes it to
// the hub. The derived Working overlay reflects the in-flight background
// mutations bracketed by BeginWork/EndWork (00 §4), so every frame emitted
// while an async op runs carries Working=true and the EndWork frame resolves
// it. The merge-eligibility overlay (CanMergeLocally/ParentBranch) is
// resolved here — off the broadcaster hot path (spec §10) — from the row's
// repo-scoped siblings, so the WorkspaceDTO carries it the moment it lands.
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	ws.Working = c.IsWorking(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	c.hub.BroadcastWorkspace(dto.WorkspaceDTOFrom(ws, elig))
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
