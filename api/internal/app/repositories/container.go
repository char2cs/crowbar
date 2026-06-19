package repositories

import (
	"context"
	"fmt"

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
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. The workspace aggregate is per-entity event-sourced: its Asynx
// instances and view DBs are resolved lazily from the adapter container by ID,
// using the injected asynxFactory (passed by the app layer to avoid an import
// cycle on newAsynx). The chat and reviewthread aggregates keep their global
// event stores and read models in the global view DB.
func New(
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axChat asynx.Asynx[domain.Chat],
	axReviewThread asynx.Asynx[domain.ReviewThread],
	asynxFactory workspace.AsynxFactory,
) (*Container, error) {
	c := &Container{hub: h}
	ws, err := workspace.New(adapters, func(w domain.Workspace) {
		c.broadcastWorkspace(context.Background(), w)
	}, asynxFactory)
	if err != nil {
		return nil, err
	}
	db := adapters.GlobalView()
	ch, err := chat.New(axChat, db, func(domain.Chat) {})
	if err != nil {
		return nil, err
	}
	rt, err := reviewthread.New(axReviewThread, db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	c.Chat = ch
	c.ReviewThread = rt
	return c, nil
}

// broadcastWorkspace converts a workspace row to its wire DTO and pushes it to
// the hub. The derived Working overlay is always false: the agent-run concept
// has been removed and the chat usecase that would set it is a dormant TODO
// (00 §5). The merge-eligibility overlay (CanMergeLocally/ParentBranch) is
// resolved here — off the broadcaster hot path (spec §10) — from the row's
// repo-scoped siblings, so the WorkspaceDTO carries it the moment it lands.
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	ws.Working = false
	elig := c.eligibilityFor(ctx, ws)
	c.hub.BroadcastWorkspace(dto.WorkspaceDTOFrom(ws, elig))
}

// eligibilityFor resolves the merge-eligibility overlay for ws by reading its
// repo-scoped siblings and applying the §10 rule: ParentID set AND a sibling
// matches that id AND its status is neither locked nor deleted → eligible with
// the parent's branch; otherwise the zero overlay. The sibling read is best
// effort — a failed List degrades to no eligibility rather than dropping the
// broadcast.
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
	for _, s := range rows {
		if s.ProjectID != ws.ProjectID || s.RepoID != ws.RepoID {
			continue
		}
		if s.ID != ws.ParentID {
			continue
		}
		eligible := s.Status != domain.WorkspaceStatusLocked &&
			s.Status != domain.WorkspaceStatusDeleted
		return wsusecase.MergeEligibility{
			CanMergeLocally: eligible,
			ParentBranch:    s.Branch,
		}
	}
	return wsusecase.MergeEligibility{}
}

// ListWorkspaces returns every workspace row. The working overlay has been
// removed (00 §5); Working is always false until the chat usecase is
// implemented. It backs the Workspaces snapshot-on-subscribe.
func (c *Container) ListWorkspaces(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces: %w", err)
	}
	for i := range rows {
		rows[i].Working = false
	}
	return rows, nil
}
