package workspace

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// WaitQuiescentForTest blocks until the repo's asynx instance has drained its
// dispatch queue and run every projection handler (WaitPublish = dispatcher
// WaitIdle + bus WaitForHandlers). It is the deterministic read-your-writes
// barrier: Send returns before the async store projection is updated (decision 4),
// so an external test calls this after a mutation to read the projection with no
// polling and no timeouts.
func WaitQuiescentForTest(repo Workspace) {
	repo.(*workspace).ax.WaitPublish()
}

// legacyCreate writes a workspace exactly as given — the shape of a row
// written before a field existed — bypassing CreateWorkspace's validation.
type legacyCreate struct{ ws domain.Workspace }

func (c legacyCreate) AggregateID() string                          { return c.ws.ID }
func (c legacyCreate) EventName() string                            { return "workspace.created." + c.ws.ID }
func (c legacyCreate) ShouldSnapshot() bool                         { return true }
func (legacyCreate) Validate(*domain.Workspace) error               { return nil }
func (c legacyCreate) EmitEvent(*domain.Workspace) domain.Workspace { return c.ws }

// WriteLegacyRowForTest records ws as history written before
// domain.Workspace.Provisioning existed has it.
func WriteLegacyRowForTest(ctx context.Context, repo Workspace, ws domain.Workspace) error {
	w, ok := repo.(*workspace)
	if !ok {
		return errors.New("not the event-sourced workspace repository")
	}
	_, err := w.ax.SendWait(ctx, legacyCreate{ws: ws})
	return err
}
