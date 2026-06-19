package v0

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// workspacesSnapshot builds the Workspaces snapshot-on-subscribe source (03 §1a):
// every workspace row, with the derived working overlay always false now the
// agent-run concept is removed, alongside the persisted hasConflicts. Each
// client's projectId/repoId predicate filters the snapshot down to its
// subscription scope.
func workspacesSnapshot(
	appContainer *app.Container,
) func() []domain.Workspace {
	return func() []domain.Workspace {
		rows, err := appContainer.Repositories.Workspace.List(context.Background())
		if err != nil {
			return nil
		}
		return rows
	}
}

// gitSnapshot builds the Git snapshot-on-subscribe source (03 §1a): the current
// GitStatus per workspace as the wsId-scoped GitStatusEvent the live broadcaster
// uses. Each client's wsId predicate filters the snapshot down to its workspace.
func gitSnapshot(
	appContainer *app.Container,
) func() []gitdomain.GitStatusEvent {
	return func() []gitdomain.GitStatusEvent {
		ctx := context.Background()
		rows, err := appContainer.Repositories.Workspace.List(ctx)
		if err != nil {
			return nil
		}
		events := make([]gitdomain.GitStatusEvent, 0, len(rows))
		for _, row := range rows {
			events = appendGitStatus(ctx, appContainer, events, row.ID)
		}
		return events
	}
}

func appendGitStatus(
	ctx context.Context,
	appContainer *app.Container,
	events []gitdomain.GitStatusEvent,
	wsID string,
) []gitdomain.GitStatusEvent {
	status, err := appContainer.Usecases.Git.Status(ctx, wsID)
	if err != nil {
		return events
	}
	// A clean tree carries a nil Files slice; normalise so the snapshot frame
	// serialises with files: [] (never null), matching the REST DTO.
	if status.Files == nil {
		status.Files = []gitdomain.GitFile{}
	}
	return append(events, gitdomain.GitStatusEvent{WsID: wsID, Status: status})
}

// lspSnapshot builds the LSP snapshot-on-subscribe source (03 §1a): the current
// diagnostics per workspace from the LSP engine's in-memory snapshot. It is
// empty until documents are opened. Each client's wsId predicate filters the
// snapshot down to its workspace.
func lspSnapshot(
	appContainer *app.Container,
	engContainer *engine.Container,
) func() []lspdomain.DiagnosticsEvent {
	if engContainer == nil || engContainer.LSP == nil {
		return nil
	}
	return func() []lspdomain.DiagnosticsEvent {
		ctx := context.Background()
		rows, err := appContainer.Repositories.Workspace.List(ctx)
		if err != nil {
			return nil
		}
		events := make([]lspdomain.DiagnosticsEvent, 0, len(rows))
		for _, row := range rows {
			events = appendDiagnostics(engContainer, events, row.ID)
		}
		return events
	}
}

func appendDiagnostics(
	engContainer *engine.Container,
	events []lspdomain.DiagnosticsEvent,
	wsID string,
) []lspdomain.DiagnosticsEvent {
	diags := engContainer.LSP.DiagnosticsSnapshot(wsID)
	if len(diags) == 0 {
		return events
	}
	return append(events, lspdomain.DiagnosticsEvent{WsID: wsID, Diagnostics: diags})
}
