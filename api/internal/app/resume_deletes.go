package app

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type pendingDeletes interface {
	PendingDeletes(ctx context.Context) ([]project.PendingDelete, error)
}

type bootRunner interface {
	Run(ctx context.Context, spec operation.Spec, work operation.Work) (domain.Operation, error)
}

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
