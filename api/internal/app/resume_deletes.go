package app

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// pendingDeletes is the delete usecase's list of unfinished deletes.
type pendingDeletes interface {
	PendingDeletes(ctx context.Context) ([]project.PendingDelete, error)
}

// bootRunner runs an operation on the caller's goroutine.
type bootRunner interface {
	Run(ctx context.Context, spec operation.Spec, work operation.Work) (domain.Operation, error)
}

// resumeDeletes finishes every delete a crash or a failure stopped, each as an
// operation that has settled when this returns. A delete that fails again is
// that operation's failure; only a failure to record one stops the boot.
func resumeDeletes(
	ctx context.Context,
	deletes pendingDeletes,
	ops bootRunner,
) error {
	pending, err := deletes.PendingDeletes(ctx)
	if err != nil {
		return fmt.Errorf("app: resume deletes: %w", err)
	}
	for _, d := range pending {
		spec := operation.Spec{
			Kind: d.Kind, Subject: d.Ref, ProjectID: d.ProjectID, Label: d.Label,
			Items: []domain.OperationItem{{Ref: d.Ref, Label: d.Label, State: domain.OpRunning}},
		}
		if _, err := ops.Run(ctx, spec, func(ctx context.Context, _ operation.Progress) error { return d.Run(ctx) }); err != nil {
			return fmt.Errorf("app: resume %s of %s: %w", d.Kind, d.Ref.ID, err)
		}
	}
	return nil
}
