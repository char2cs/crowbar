package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func purgeGaveUp(
	ops *operation.Service,
) func(ctx context.Context, wsID string, tomb *domain.Workspace, err error) {
	return func(ctx context.Context, wsID string, tomb *domain.Workspace, cause error) {
		spec := operation.Spec{
			Kind:    domain.OpWorkspacePurge,
			Subject: domain.EntityRef{Kind: domain.EntityWorkspace, ID: wsID},
			Label:   "Remove workspace",
		}
		reason := fmt.Errorf("%w; its files may still be on disk", cause)
		if tomb != nil {
			spec.Label, spec.ProjectID = "Remove "+tomb.Branch, tomb.ProjectID
			reason = fmt.Errorf("%w; Crowbar retries when it next starts", cause)
		}
		if _, err := ops.Fail(ctx, spec, reason); err != nil {
			slog.ErrorContext(ctx, "app: record the purge that gave up", "workspace", wsID, "cause", cause, "err", err)
		}
	}
}

func purgeFinished(ops *operation.Service) func(ctx context.Context, wsID string) {
	return func(ctx context.Context, wsID string) {
		ref := domain.EntityRef{Kind: domain.EntityWorkspace, ID: wsID}
		if err := ops.DismissFailed(ctx, domain.OpWorkspacePurge, ref); err != nil {
			slog.ErrorContext(ctx, "app: clear the purge the sweep finished", "workspace", wsID, "err", err)
		}
	}
}
