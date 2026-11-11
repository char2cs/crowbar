package app

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// parkedReport hears an aggregate whose durable delivery parked.
type parkedReport[T any] func(ctx context.Context, evt asynxModels.Event[T], cause error)

// reportParked records a parked aggregate as a failed operation on its entity:
// until a restart redelivers it, every later change to it is refused, and the
// user is owed the reason. One with no entity a user sees (zero ref) is logged.
func reportParked[T any](
	ops *operation.Service,
	what string,
	subject func(asynxModels.Event[T]) (domain.EntityRef, string),
) parkedReport[T] {
	return func(ctx context.Context, evt asynxModels.Event[T], cause error) {
		ref, projectID := subject(evt)
		if ref.ID == "" {
			slog.ErrorContext(ctx, "app: delivery parked", "what", what, "aggregate", evt.AggregateID, "cause", cause)
			return
		}
		spec := operation.Spec{Kind: domain.OpEntitySave, Subject: ref, ProjectID: projectID, Label: "Save " + what}
		reason := &problem.Error{
			Code:    problem.Unavailable,
			Message: "Crowbar stopped saving this " + what + "'s changes until it restarts",
			Err:     cause,
		}
		if _, err := ops.Fail(context.WithoutCancel(ctx), spec, reason); err != nil {
			slog.ErrorContext(ctx, "app: record a parked delivery", "aggregate", evt.AggregateID, "cause", cause, "err", err)
		}
	}
}

// parkedReports are the reporters of each aggregate type, by the entity it saves.
type parkedReports struct {
	workspace    parkedReport[domain.Workspace]
	reviewThread parkedReport[domain.ReviewThread]
	agentChat    parkedReport[domain.Chat]
	tree         parkedReport[tree.Tree]
}

func newParkedReports(ops *operation.Service) parkedReports {
	return parkedReports{
		workspace: reportParked(ops, "workspace", func(e asynxModels.Event[domain.Workspace]) (domain.EntityRef, string) {
			return domain.EntityRef{Kind: domain.EntityWorkspace, ID: e.AggregateID}, e.Aggregate.ProjectID
		}),
		// A review thread is shown on, and saved with, its workspace.
		reviewThread: reportParked(ops, "review thread", func(e asynxModels.Event[domain.ReviewThread]) (domain.EntityRef, string) {
			return domain.EntityRef{Kind: domain.EntityWorkspace, ID: e.Aggregate.WsID}, ""
		}),
		agentChat: reportParked(ops, "chat", func(e asynxModels.Event[domain.Chat]) (domain.EntityRef, string) {
			return domain.EntityRef{Kind: domain.EntityChat, ID: e.AggregateID}, ""
		}),
		// The sidebar tree is one per project, keyed by it; the unplaced one is no project's.
		tree: reportParked(ops, "sidebar", func(e asynxModels.Event[tree.Tree]) (domain.EntityRef, string) {
			if e.AggregateID == tree.Unplaced {
				return domain.EntityRef{}, ""
			}
			return domain.EntityRef{Kind: domain.EntityProject, ID: e.AggregateID}, e.AggregateID
		}),
	}
}
