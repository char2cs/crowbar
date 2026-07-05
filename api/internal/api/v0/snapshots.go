package v0

import (
	"context"
	"strings"
	"time"

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
// §10 rule, and the derived working overlay (via ListWorkspaces) so a client
// subscribing mid-mutation sees the in-flight state immediately.
func workspacesSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.WorkspaceDTO {
	return func(scope string) []dto.WorkspaceDTO {
		ctx := context.Background()
		projectID, repoID := parseRepoScope(scope)
		siblings, err := appContainer.Repositories.ListWorkspacesInRepo(ctx, projectID, repoID)
		if err != nil {
			return nil
		}
		// Snapshot-on-subscribe has no request to scope to (it's built lazily for
		// a connecting client), so it owns a background context — the same one it
		// already uses for the List above. The detached context is a visible,
		// edge-level choice here, not hidden inside the usecase.
		eligFn := func(w domain.Workspace) workspace.MergeEligibility {
			return appContainer.Usecases.Workspace.MergeEligibilityFor(context.Background(), w, siblings)
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

// threadsSnapshot builds the Threads snapshot-on-subscribe source (03 §1a) from
// the global ReviewThread aggregate, scoped to the single workspace parsed from
// the connecting client's hierarchical subscription prefix (p/r/w). It resolves
// the wsID from the scope and lists only that workspace's threads via
// ListByWorkspace — never a global enumeration — stamping the project/repo from
// the scope onto each ThreadDTO. A scope without a workspace segment (a repo- or
// project-level subscription) yields nil, since threads are workspace-scoped.
func threadsSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.ThreadDTO {
	return func(scope string) []dto.ThreadDTO {
		parts := strings.Split(scope, "/")
		if len(parts) < 3 || parts[2] == "" {
			return nil
		}
		projectID, repoID, wsID := parts[0], parts[1], parts[2]
		rows, err := appContainer.Repositories.ReviewThread.ListByWorkspace(
			context.Background(),
			wsID,
		)
		if err != nil {
			return nil
		}
		return dto.ThreadDTOList(rows, projectID, repoID)
	}
}

// scopedWorkspaceRows resolves scope to the workspaces gitSnapshot/lspSnapshot
// should cover. The broadcaster (ws/broadcaster.go Handle) always invokes
// Snapshot with clientScope's full hierarchical "p/r/w" prefix — never the bare
// id ScopeKey/scopeWsID resolves for the separate OnSubscribe/OnUnsubscribe
// lifecycle hooks (watcher/LSP/origin-sync refcounting) — so scope here is
// parsed exactly like threadsSnapshot/terminalsSnapshot parse it: the third
// segment is the workspace id. A scope with fewer than 3 segments (or callers,
// such as unit tests, that pass a bare workspace id directly with no "/") is
// treated as already being the workspace id verbatim, so a direct call like
// gitSnapshot(a)("w1") still resolves. The resolved workspace's repo siblings
// are returned; the broadcaster's own wsId predicate filters delivery down to
// the connecting client's exact workspace afterward, exactly as it already
// does today. A blank scope (a list-level subscribe — not currently used by
// either broadcaster, but handled defensively) falls back to every workspace.
// An unresolvable scope (unknown workspace id) yields no rows rather than an
// error, since a snapshot degrading to empty is safe and a stale/racing
// subscribe for an already-deleted workspace is expected, not exceptional.
func scopedWorkspaceRows(
	ctx context.Context,
	appContainer *app.Container,
	scope string,
) ([]domain.Workspace, error) {
	if scope == "" {
		return appContainer.Repositories.ListWorkspaces(ctx)
	}
	wsID := scope
	if parts := strings.Split(scope, "/"); len(parts) >= 3 && parts[2] != "" {
		wsID = parts[2]
	}
	ws, err := appContainer.Repositories.Workspace.Get(ctx, wsID)
	if err != nil {
		return nil, nil
	}
	return appContainer.Repositories.ListWorkspacesInRepo(ctx, ws.ProjectID, ws.RepoID)
}

// gitSnapshot builds the Git snapshot-on-subscribe source (03 §1a): the current
// GitStatus per workspace as the wsId-scoped GitStatusEvent the live broadcaster
// uses. Each client's wsId predicate filters the snapshot down to its workspace.
func gitSnapshot(
	appContainer *app.Container,
) func(scope string) []gitdomain.GitStatusEvent {
	return func(scope string) []gitdomain.GitStatusEvent {
		ctx := context.Background()
		rows, err := scopedWorkspaceRows(ctx, appContainer, scope)
		if err != nil {
			return nil
		}
		// scopedWorkspaceRows returns a literal nil (not merely empty) for an
		// unresolvable scope; preserve that nil rather than let the make() below
		// paper over it with a non-nil empty slice, so an unknown/racing
		// workspace id degrades the snapshot exactly like a list error does.
		if rows == nil {
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
	return func(scope string) []lspdomain.DiagnosticsEvent {
		ctx := context.Background()
		rows, err := scopedWorkspaceRows(ctx, appContainer, scope)
		if err != nil {
			return nil
		}
		// See the matching guard in gitSnapshot: preserve scopedWorkspaceRows'
		// literal nil for an unresolvable scope instead of upgrading it to a
		// non-nil empty slice via make().
		if rows == nil {
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

// terminalsSnapshot builds the Terminal-session snapshot-on-subscribe source
// (03 §1a) from the in-memory engine registry (D6: terminals are ephemeral, no
// view.db). Every live session across every workspace is emitted with its real
// state (active|detached|suspended) carrying its workspace's project/repo scope;
// each client's hierarchical prefix predicate trims the result to its
// subscription. It is empty until a session is created.
func terminalsSnapshot(
	_ *app.Container,
	engContainer *engine.Container,
) func(scope string) []dto.TerminalSessionDTO {
	if engContainer == nil || engContainer.Terminal == nil {
		return nil
	}
	return func(scope string) []dto.TerminalSessionDTO {
		// Terminals are workspace-scoped: the subscribing client's scope is the
		// hierarchical p/r/w key. Resolve the single workspace from the scope and
		// list only its sessions — never enumerate every workspace's per-entity
		// store (the scope arg exists precisely to avoid that global scan).
		parts := strings.Split(scope, "/")
		if len(parts) < 3 || parts[2] == "" {
			return nil
		}
		projectID, repoID, wsID := parts[0], parts[1], parts[2]
		ids := engContainer.Terminal.ListSessionsForWorkspace(wsID)
		now := time.Now().UTC()
		out := make([]dto.TerminalSessionDTO, 0, len(ids))
		for _, id := range ids {
			state, ok := engContainer.Terminal.StateOf(id)
			if !ok {
				state = "active" // session vanished between List and StateOf; skip
				continue
			}
			out = append(out, dto.TerminalSessionDTOFrom(id, wsID, projectID, repoID, "", state, now))
		}
		return out
	}
}
