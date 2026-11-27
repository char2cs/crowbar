package workspace

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func WaitQuiescentForTest(repo Workspace) {
	repo.(*workspace).ax.WaitPublish()
}

type legacyCreate struct{ ws domain.Workspace }

func (c legacyCreate) AggregateID() string                          { return c.ws.ID }
func (c legacyCreate) EventName() string                            { return "workspace.created." + c.ws.ID }
func (c legacyCreate) ShouldSnapshot() bool                         { return true }
func (legacyCreate) Validate(*domain.Workspace) error               { return nil }
func (c legacyCreate) EmitEvent(*domain.Workspace) domain.Workspace { return c.ws }

func WriteLegacyRowForTest(ctx context.Context, repo Workspace, ws domain.Workspace) error {
	w, ok := repo.(*workspace)
	if !ok {
		return errors.New("not the event-sourced workspace repository")
	}
	_, err := w.ax.SendWait(ctx, legacyCreate{ws: ws})
	return err
}
