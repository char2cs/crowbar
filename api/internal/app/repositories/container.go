package repositories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/node"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

// Container holds the aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	ReviewThread reviewthread.ReviewThread
	// AgentChat is the asynx-backed EventStore: its store/hub projections are
	// live on axAgentChat and the agent usecase sends every AgentChat mutation
	// through it (the gorm-backed store was retired in the Task 10 cutover).
	AgentChat agentchat.EventStore
	// Node is the position aggregate: the one owner of every sidebar row's
	// ParentID/Order, at every tree level.
	Node node.EventStore
	// ChatLedger is each chat's append-only ledger, one writer per chat.
	ChatLedger *ledgerstore.Store
	// ChatBlobs holds the tool payloads the ledger's rows name.
	ChatBlobs *ledgerstore.Blobs
	hub       hub.WebSocketHub
	git       wsusecase.MergeConflictBatchChecker
	// PurgeChat hard-deletes one chat and everything it owns — its aggregate, the
	// CLIs on it, its conversation record and telemetry, its conversation history
	// and its ledger directory. It is the chat usecase's own PurgeChat, the SAME
	// path a user's chat delete takes, so the workspace-delete cascade cannot
	// drift from it again (spec §7-D, invariant D5): it used to keep its own copy
	// that skipped the conversation record, the telemetry and the ledger.
	//
	// Assigned after construction because the chat usecase is built from this
	// container. The cascade REFUSES to run without it rather than purging
	// half a chat.
	PurgeChat func(ctx context.Context, chatID string) error
	// axWorkspace/axReviewThread/axAgentChat/axNode are the per-type asynx
	// instances WaitQuiescent drains: the read-your-writes barrier for tests.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.Chat]
	axNode         asynx.Asynx[domain.Node]
	// inflight counts the background mutations currently running per workspace
	// id (00 §4 fail-fast/good-path-async). It backs the derived Working overlay:
	// the API layer brackets each async op with BeginWork/EndWork, and every
	// serving path (live broadcast, snapshot, REST reads) overlays IsWorking so
	// the client spinner tracks real daemon activity.
	mu       sync.Mutex
	inflight map[string]int

	// agentWorking maps a workspace id to its busy agent chats; agentBroadcasts
	// is each workspace's rebroadcast worker (agent_working.go). Guarded by mu.
	agentWorking    map[string]map[string]struct{}
	agentBroadcasts map[string]bool

	// drainWG tracks every post-commit reactor goroutine wireCallbacks registered
	// (the async delete reactor, and — once wired — the reconcile-on-open tasks) so
	// the app layer's ordered graceful shutdown (Task 15) can wait them out before
	// closing the DBs; drainCancel closes the shared drain gate to stop reactors
	// starting new work, derived from drainCtx (decisions 9 + 11). They are created
	// and stored by wireCallbacks and reached from app.Container via Drain().
	drainGate   *drain.Gate
	drainCtx    context.Context
	drainCancel context.CancelFunc
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. The workspace aggregate is backed by the singleton axWorkspace (one
// instance per type, routing every id by shard hash) built by the app layer; its
// read model lives in state/store/workspace.db.
// The reviewthread aggregate owns its central per-type read model at
// state/store/review_thread.db. The agentchat aggregate owns its central
// per-type read model at state/store/agent_chat.db. The workspace-delete
// cascade's chat purge, Container.PurgeChat, is assigned by the app layer once
// the chat usecase exists; see the field's doc comment. rm holds the read
// models the asynx instances were built to deliver to (OpenReadModels).
func New(
	ctx context.Context,
	adapters *adapter.Container,
	rm *ReadModels,
	h hub.WebSocketHub,
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	axAgentChat asynx.Asynx[domain.Chat],
	axNode asynx.Asynx[domain.Node],
	git wsusecase.MergeConflictBatchChecker,
	chatWatch agentchat.WatchFunc,
	nodeWatch node.WatchFunc,
) (*Container, error) {
	c := &Container{
		hub: h, git: git, inflight: map[string]int{},
		agentWorking: map[string]map[string]struct{}{}, agentBroadcasts: map[string]bool{},
		axWorkspace: axWorkspace, axReviewThread: axReviewThread, axAgentChat: axAgentChat,
		axNode: axNode,
	}
	ws, err := workspace.New(axWorkspace, adapters.WorkspaceES(), rm.Workspace)
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
	if err := workspace.RegisterHubProjection(c.Workspace, c.enrichFrame, c.hub.BroadcastWorkspace); err != nil {
		return nil, fmt.Errorf("repositories: workspace hub projection: %w", err)
	}
	// reviewthread owns its own central per-type read model at
	// state/store/review_thread.db (Task 12), no longer the shared view.db: pass
	// ReviewThreadView() as the read-model DB while keeping ReviewThreadES() for the
	// lazy AggregateLister Replay (§3.7).
	rt, err := reviewthread.New(axReviewThread, adapters.ReviewThreadES(), rm.ReviewThread, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.ReviewThread = rt

	// agentchat: build the asynx-backed EventStore over the singleton
	// axAgentChat, registering its store + hub projections (store.New, invoked by
	// NewEventSourced) exactly once. chatWatch is the SOLE source of
	// agent-chat lifecycle frames: every agentchat.* event the agent usecase's
	// commands emit is fanned out here by the hub projection. The usecase no
	// longer broadcasts manually (that double-broadcast was retired at cutover),
	// so this projection is the one and only WS feed for agent chats.
	agentChat, err := agentchat.NewEventSourced(axAgentChat, adapters.AgentChatES(), rm.AgentChat, chatWatch)
	if err != nil {
		return nil, fmt.Errorf("repositories: agent chat event store: %w", err)
	}
	c.AgentChat = agentChat
	if _, err := axAgentChat.OnForget(c.forgetAgentWorking); err != nil {
		return nil, fmt.Errorf("repositories: agent working overlay: %w", err)
	}

	nodeStore, err := node.NewEventSourced(axNode, adapters.NodeES(), rm.Node, nodeWatch)
	if err != nil {
		return nil, fmt.Errorf("repositories: node event store: %w", err)
	}
	c.Node = nodeStore
	if c.ChatLedger, err = ledgerstore.Open(ctx, adapters.ChatLedgerDB(), time.Now); err != nil {
		return nil, fmt.Errorf("repositories: chat ledger: %w", err)
	}
	// Tool payloads live beside the state directory, swept by its retention.
	if c.ChatBlobs, err = ledgerstore.OpenBlobs(adapters.Home().Content()); err != nil {
		return nil, fmt.Errorf("repositories: chat payloads: %w", err)
	}

	// Wire the post-commit cross-aggregate reactions (spec §3.6): the workspace
	// delete reactor + its review-thread AND agent-chat forget cascades, all
	// joined to the shared drain WaitGroup so graceful shutdown can quiesce them
	// (Task 15). Every cascaded repo must already be built — forgetDependents
	// calls into c.ReviewThread and c.AgentChat.
	if err := c.wireCallbacks(ctx, adapters.Home().Root()); err != nil {
		return nil, fmt.Errorf("repositories: wire callbacks: %w", err)
	}
	return c, nil
}

// WaitQuiescent blocks until every per-type asynx instance has drained its
// dispatch queue and run all projection handlers (WaitPublish = dispatcher
// WaitIdle + bus WaitForHandlers). It is the deterministic read-your-writes
// barrier: production uses the async Send path, so the store/list read model and
// the hub broadcast are INDEPENDENT projections that settle out of band; a test
// calls WaitQuiescent after a mutation so a subsequent read of ANY projection is
// guaranteed consistent — with no polling and no timeouts.
func (c *Container) WaitQuiescent() {
	c.axWorkspace.WaitPublish()
	c.axReviewThread.WaitPublish()
	c.axAgentChat.WaitPublish()
	c.axNode.WaitPublish()
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
	ws.Working = c.WorkingFor(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	return dto.WorkspaceDTOFrom(ctx, ws, elig, c.owningChatIDFor(ctx, ws), c.nodePlacement(ctx, ws))
}

// nodePlacement resolves ws's own sidebar placement for the wire DTO. An
// ordinary fork's placement lives on its OWNING CHAT's own ParentID/Order —
// PlaceWorkspace's write for exactly this case (place_workspace.go's nodeID
// doc) lands there via Chats.SetPlacement/SetOrder, a real AgentChat
// aggregate field, never a Node row: no Node is ever minted for a plain
// chat, so reading one back via this container's Node store always missed,
// silently degrading to "" / 0 regardless of how long ago the drag landed.
// Caught live: a fork dragged into a folder showed it there for a moment,
// then reseeded straight back to the repo root on the very next broadcast —
// the PATCH response's own echoed placement was right, only this read (fed
// straight into every subsequent WS frame) was wrong. Only a LOCKED branch,
// whose owning chat carries no Node of its own, is genuinely addressed by
// ws.ID's own Node{Kind:workspace} row — the one case the Node store below
// still answers.
//
// Resolved eagerly here, once, rather than inside Placement: enrichFrame
// already holds ws and calls this exactly once per frame, and
// owningChatIDFor's own resolution is the identical one this needs — no
// second, independently-drifting copy.
func (c *Container) nodePlacement(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspacePlacementReader {
	if !ws.RendersAsBranch() {
		if owner, ok := c.resolveOwningChat(ctx, ws); ok {
			return chatPlacementReader{parentID: owner.ParentID, order: owner.Order}
		}
	}
	if c.Node == nil {
		return nil
	}
	return nodePlacementReader{nodes: c.Node, nodeID: ws.ID}
}

// chatPlacementReader answers a resolved owning chat's own ParentID/Order
// directly — no store read of its own, since the caller already resolved
// the chat this frame needs.
type chatPlacementReader struct {
	parentID string
	order    int
}

func (r chatPlacementReader) Placement(
	_ context.Context,
	_ string,
) (folderID string, order int) {
	return r.parentID, r.order
}

type nodePlacementReader struct {
	nodes  node.EventStore
	nodeID string
}

func (r nodePlacementReader) Placement(
	ctx context.Context,
	_ string,
) (folderID string, order int) {
	n, err := r.nodes.GetNode(ctx, r.nodeID)
	if err != nil {
		return "", 0
	}
	return n.ParentID, n.Order
}

// owningChatIDFor resolves wsID's real owning chat id for the wire DTO,
// reusing domain.ResolveOwningChat over this container's own AgentChat read —
// never a second, independently derived answer. An unwired AgentChat (a test
// Container built with only the fields its own assertion needs, matching
// eligibilityFor's own zero-value tolerance below) or an unresolvable read
// degrades to "".
//
// Deliberately left resolving through the CHAT side, unchanged, by 2026-09-08
// sidebar-placement-unification Task 7: every workspace now also mints its own
// Node row (ID == ws.ID), but dozens of live frontend call sites still address
// a workspace's sidebar position through THIS chat id (see
// web/src/components/sidebar/lib/branch-row-id.ts), not ws.ID — repointing it
// here with no frontend migration would break them. Task 9 deleted the
// backfill machinery that used to MAINTAIN this chat id (a fresh workspace's
// owning chat is minted chat-first at creation regardless, independent of
// that machinery); the frontend migration, and retiring this field, is still
// pending.
func (c *Container) owningChatIDFor(
	ctx context.Context,
	ws domain.Workspace,
) string {
	owner, ok := c.resolveOwningChat(ctx, ws)
	if !ok {
		return ""
	}
	return owner.ID
}

// resolveOwningChat resolves ws's real owning chat ROW (not just its id),
// reusing domain.ResolveOwningChat over this container's own AgentChat read —
// never a second, independently derived answer. An unwired AgentChat (a test
// Container built with only the fields its own assertion needs) or an
// unresolvable read degrades to no owner.
func (c *Container) resolveOwningChat(
	ctx context.Context,
	ws domain.Workspace,
) (domain.Chat, bool) {
	if c.AgentChat == nil {
		return domain.Chat{}, false
	}
	rows, err := c.AgentChat.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return domain.Chat{}, false
	}
	return domain.ResolveOwningChat(rows)
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

// WorkingFor reports whether the workspace is working via EITHER derived overlay:
// a background mutation in flight (inflight, via IsWorking) OR an agent chat
// mid-turn (agentWorking, via agentWorkingFor). It is the single combined read the
// REST list/detail handlers stamp Working from, so a REST read agrees with both
// the live broadcast frames (enrichFrame) and the snapshot-on-subscribe readers
// (ListWorkspaces/ListWorkspacesInRepo) — all four converge on this method. Each
// overlay is read under its OWN lock acquisition (never one lock held across
// both), mirroring the existing enrichFrame combination: the two are independent
// booleans with no cross-overlay invariant, so a transient interleaving can only
// observe a value that was truthful at some instant during the call — the same
// guarantee every other reader already provides.
func (c *Container) WorkingFor(
	wsID string,
) bool {
	return c.IsWorking(wsID) || c.agentWorkingFor(wsID)
}

// rebroadcast pushes the workspace's current row to the hub so an overlay
// transition (Begin/EndWork) is visible without waiting for the op's next
// event. Best-effort: a missing row (already deleted) skips silently.
//
// A read failure here is LOGGED rather than dropped in silence, because the frame
// it costs us is not interchangeable with the next one. Working is transported as
// a delta and nothing repeats it: the transition that turns the spinner OFF is
// broadcast exactly once, so a Get that fails leaves every client spinning over an
// idle workspace until some unrelated event happens to re-broadcast the row. That
// wedge is invisible from the outside — the daemon's own state is correct and the
// event log is clean — and it stayed invisible precisely because this returned
// without a word. ErrNotFound is the expected case (the op deleted its own row)
// and is not worth a line.
func (c *Container) rebroadcast(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		if !errors.Is(err, apperr.ErrNotFound) {
			slog.WarnContext(ctx, "repositories: rebroadcast: get workspace "+
				"(client overlay may be stale until the next event)",
				"workspace_id", wsID, "err", err)
		}
		return
	}
	c.broadcastWorkspace(ctx, ws)
}

// eligibilityFor resolves the merge-eligibility overlay (incl. the predicted
// merge-conflict flag) for ws by reading its siblings and delegating to the
// shared wsusecase.ResolveMergeEligibilities — the SAME resolver the snapshot read
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
	siblings, err := c.ListWorkspacesInRepo(ctx, ws.ProjectID, ws.RepoID)
	if err != nil {
		return wsusecase.MergeEligibility{}
	}
	return wsusecase.ResolveMergeEligibilities(ctx, []domain.Workspace{ws}, siblings, c.git)[0]
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
		rows[i].Working = c.WorkingFor(rows[i].ID)
	}
	return rows, nil
}

// ListWorkspacesInRepo returns every workspace row scoped to one project+repo,
// with the derived Working overlay applied, read from MY central store read
// model (state/store/workspace.db) filtered by project_id/repo_id via the
// workspace repo's ListInRepo — a single central-store read, not the
// whole-install per-entity scan the retired workspace_directory projection was
// built to avoid. It backs the repo-scoped snapshot-on-subscribe builders and
// the merge-eligibility overlay.
func (c *Container) ListWorkspacesInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces in repo: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.WorkingFor(rows[i].ID)
	}
	return rows, nil
}
