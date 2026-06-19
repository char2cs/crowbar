package v0

import (
	"context"
	"strings"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// workspacesSnapshot builds the Workspaces snapshot-on-subscribe source (03 §1a)
// as wire DTOs, scoped to the repo parsed from the connecting client's
// subscription prefix ("p/r/..."). Each row carries the merge-eligibility
// overlay (CanMergeLocally/ParentBranch) computed from its repo siblings via the
// §10 rule. The derived working overlay is always false now the agent-run
// concept is removed.
func workspacesSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.WorkspaceDTO {
	return func(scope string) []dto.WorkspaceDTO {
		projectID, repoID := parseRepoScope(scope)
		rows, err := appContainer.Repositories.Workspace.List(context.Background())
		if err != nil {
			return nil
		}
		siblings := scopeWorkspacesToRepo(rows, projectID, repoID)
		eligFn := func(w domain.Workspace) workspace.MergeEligibility {
			return appContainer.Usecases.Workspace.MergeEligibilityFor(w, siblings)
		}
		return dto.WorkspaceDTOList(siblings, eligFn)
	}
}

// parseRepoScope splits a hierarchical subscription prefix ("p", "p/r", or
// "p/r/w") into its projectID and repoID. A scope with fewer segments yields
// empty components, which scopeWorkspacesToRepo treats as "match all" so a
// project-level or global subscription still snapshots its subtree.
func parseRepoScope(
	scope string,
) (string, string) {
	if scope == "" {
		return "", ""
	}
	segs := strings.Split(scope, "/")
	projectID := segs[0]
	repoID := ""
	if len(segs) > 1 {
		repoID = segs[1]
	}
	return projectID, repoID
}

// scopeWorkspacesToRepo filters rows to those matching the given projectID and
// repoID. An empty component matches every value at that level, so a
// project-level scope keeps all of a project's repos and an empty scope keeps
// every row.
func scopeWorkspacesToRepo(
	rows []domain.Workspace,
	projectID string,
	repoID string,
) []domain.Workspace {
	if projectID == "" && repoID == "" {
		return rows
	}
	out := make([]domain.Workspace, 0, len(rows))
	for _, w := range rows {
		if projectID != "" && w.ProjectID != projectID {
			continue
		}
		if repoID != "" && w.RepoID != repoID {
			continue
		}
		out = append(out, w)
	}
	return out
}

// projectSnapshot builds the Projects snapshot-on-subscribe source (03 §1a) as
// wire DTOs. Projects sit at the top of the hierarchy, so the scope is either
// empty (list-level subscription) or a bare project id (project-level): either
// way the snapshot returns the full project set and the per-client prefix
// predicate filters it down. A failed list degrades to a nil snapshot.
func projectSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.ProjectDTO {
	return func(_ string) []dto.ProjectDTO {
		rows, err := appContainer.GORM.Projects.FindAll(context.Background())
		if err != nil {
			return nil
		}
		return dto.ProjectDTOList(rows)
	}
}

// repoSnapshot builds the Repos snapshot-on-subscribe source (03 §1a) as wire
// DTOs, scoped to the project parsed from the connecting client's subscription
// prefix ("p/..."). An empty project component matches every project, so a
// list-level subscription snapshots every repo. A failed list degrades to a nil
// snapshot.
func repoSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.RepoDTO {
	return func(scope string) []dto.RepoDTO {
		projectID, _ := parseRepoScope(scope)
		rows, err := appContainer.GORM.Repositories.FindAll(context.Background())
		if err != nil {
			return nil
		}
		return dto.RepoDTOList(scopeReposToProject(rows, projectID))
	}
}

// scopeReposToProject filters rows to those under the given projectID. An empty
// projectID matches every repo, so a list-level scope keeps the full set.
func scopeReposToProject(
	rows []domain.Repository,
	projectID string,
) []domain.Repository {
	if projectID == "" {
		return rows
	}
	out := make([]domain.Repository, 0, len(rows))
	for _, r := range rows {
		if r.ProjectID != projectID {
			continue
		}
		out = append(out, r)
	}
	return out
}

// gitSnapshot builds the Git snapshot-on-subscribe source (03 §1a): the current
// GitStatus per workspace as the wsId-scoped GitStatusEvent the live broadcaster
// uses. Each client's wsId predicate filters the snapshot down to its workspace.
func gitSnapshot(
	appContainer *app.Container,
) func(scope string) []gitdomain.GitStatusEvent {
	return func(_ string) []gitdomain.GitStatusEvent {
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
) func(scope string) []lspdomain.DiagnosticsEvent {
	if engContainer == nil || engContainer.LSP == nil {
		return nil
	}
	return func(_ string) []lspdomain.DiagnosticsEvent {
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
