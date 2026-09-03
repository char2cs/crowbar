package v0

import (
	"context"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
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
		owningChatIDFn := func(w domain.Workspace) string {
			return resolveOwningChatID(context.Background(), appContainer, w.ID)
		}
		return dto.WorkspaceDTOList(siblings, eligFn, owningChatIDFn)
	}
}

// resolveOwningChatID answers wsID's real owning chat id for the wire DTO,
// reusing Task 3's own branch-preferring resolution
// (agentusecase.ResolveOwningChat) over the chat usecase's own read of the
// workspace's chat rows — never a second, independently derived answer. An
// unresolvable read (or a workspace this backfill has not reached yet)
// degrades to "".
func resolveOwningChatID(
	ctx context.Context,
	appContainer *app.Container,
	wsID string,
) string {
	rows, err := appContainer.Usecases.AgentChat.ListChatsByWorkspace(ctx, wsID)
	if err != nil {
		return ""
	}
	owner, ok := agentusecase.ResolveOwningChat(rows)
	if !ok {
		return ""
	}
	return owner.ID
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

// scopedWorkspaceRows resolves a HIERARCHICAL scope to the workspace(s)
// gitSnapshot/lspSnapshot should cover. For a workspace-scoped route the
// broadcaster (ws/broadcaster.go Handle) invokes Snapshot with clientScope's
// full "p/r/w" prefix — never the bare id ScopeKey/scopeWsID resolves for the
// separate OnSubscribe/OnUnsubscribe lifecycle hooks (watcher/LSP/origin-sync
// refcounting) — so scope here is parsed exactly like
// threadsSnapshot/terminalsSnapshot parse it: the third segment is the
// workspace id. A scope with fewer than 3 segments is treated as already being
// the workspace id verbatim, which is what a caller passing one directly
// (either snapshot's own unit tests) means by it.
//
// The bare form no longer reaches here from GIT or LSP: a bare scope on either
// topic is a CHAT id now, and gitSnapshot/lspSnapshot each resolve it — via
// their own chatGitSnapshot/chatLSPSnapshot — before this function is reached.
// See their doc comments.
//
// Only the resolved workspace is returned — not its repo siblings — because
// gitDef/lspDef scope their WS subscription to exactly one wsId (or chatId) with
// an exact-match predicate (container.go): every event for any other workspace
// is discarded after delivery, so computing git status / diagnostics for
// siblings would be wasted work on this exact tab-open hot path. An
// unresolvable scope (unknown workspace id) yields no rows rather than an
// error, since a snapshot degrading to empty is safe and a stale/racing
// subscribe for an already-deleted workspace is expected, not exceptional. A
// blank scope (a list-level subscribe — not currently used by either
// broadcaster, but handled defensively) falls back to every workspace.
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
	return []domain.Workspace{ws}, nil
}

// gitSnapshot builds the Git snapshot-on-subscribe source (03 §1a): the current
// GitStatus for the subscribing client's worktree, as the same GitStatusEvent
// the live broadcaster carries, so the client's own predicate filters the
// replay exactly the way it filters live frames.
//
// It answers the TWO scope shapes git's two live routes produce, and it can
// tell them apart because they are shaped differently, not because it is told
// which mount it came from (a snapshot is built for a connecting client, with
// no handler and no route in sight — see ws.Broadcaster.Handle):
//
//   - HIERARCHICAL ("p/r/w"), from the workspace-scoped route: ws.clientScope
//     joins its projectId/repoId/wsId path params, and the workspace is the
//     third segment. Unchanged from before this step.
//   - BARE (a single segment), from /v0/chats/:chatId/git/status: that route
//     binds none of those three, so ws.clientScope falls back to the bare chat
//     id and the worktree has to be RESOLVED from it (spec §3), exactly as the
//     route's own middleware resolved it for the REST handlers.
//
// A bare id is therefore a CHAT id here, never a workspace id. It used to be
// read as the latter, as an affordance for unit tests calling this directly;
// that affordance is gone rather than kept beside the new meaning, because one
// string cannot honestly mean both and the real broadcaster never produced it.
func gitSnapshot(
	appContainer *app.Container,
) func(scope string) []gitdomain.GitStatusEvent {
	return func(scope string) []gitdomain.GitStatusEvent {
		ctx := context.Background()
		if chatID, ok := bareChatScope(scope); ok {
			return chatGitSnapshot(ctx, appContainer, chatID)
		}
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

// bareChatScope reports whether scope is the flat, single-segment form the
// chat-scoped route produces, and returns the chat id it names. An empty scope
// is not one: it is the list-level subscribe scopedWorkspaceRows already
// handles defensively.
func bareChatScope(
	scope string,
) (string, bool) {
	if scope == "" || strings.Contains(scope, "/") {
		return "", false
	}
	return scope, true
}

// chatGitSnapshot replays the worktree behind chatID. A chat whose worktree
// cannot be resolved — no worktree anywhere in its ancestry, or a racing delete
// — yields nil, the same degradation an unresolvable workspace scope takes:
// the connection still opens and simply replays nothing.
func chatGitSnapshot(
	ctx context.Context,
	appContainer *app.Container,
	chatID string,
) []gitdomain.GitStatusEvent {
	if appContainer == nil || appContainer.Usecases == nil || appContainer.Usecases.Worktree == nil {
		return nil
	}
	workspace, err := appContainer.Usecases.Worktree.Resolve(ctx, chatID)
	if err != nil {
		return nil
	}
	return appendGitStatus(ctx, appContainer, make([]gitdomain.GitStatusEvent, 0, 1), workspace.ID)
}

// appendGitStatus appends wsID's current status as the fully-scoped event.
//
// It stamps the fan-out set for the same reason PushGit does: a snapshot frame
// and a live frame are filtered by the SAME compiled predicate, so a replay
// that carried no chat ids would be silently dropped for exactly the
// chat-scoped clients this step exists to serve — a connection that opens,
// replays nothing, and only comes alive on the next file change.
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
	return append(events, gitdomain.GitStatusEvent{
		WsID:    wsID,
		ChatIDs: snapshotChatsHolding(ctx, appContainer, wsID),
		Status:  status,
	})
}

// snapshotChatsHolding resolves wsID's fan-out set for a replay frame,
// degrading to no chats rather than an error — the same way every other read on
// this path degrades a snapshot instead of failing a subscribe.
func snapshotChatsHolding(
	ctx context.Context,
	appContainer *app.Container,
	wsID string,
) []string {
	if appContainer.Usecases == nil || appContainer.Usecases.Worktree == nil {
		return nil
	}
	chatIDs, err := appContainer.Usecases.Worktree.ChatsForWorkspace(ctx, wsID)
	if err != nil {
		return nil
	}
	return chatIDs
}

// lspSnapshot builds the LSP snapshot-on-subscribe source (03 §1a): the current
// diagnostics for the subscribing client's own LSP session, as the same
// DiagnosticsEvent shape the live broadcaster carries, so the client's own
// predicate filters the replay exactly the way it filters live frames.
//
// It answers the TWO scope shapes LSP's two live routes produce, told apart the
// same way gitSnapshot tells its two apart (no handler, no route in sight —
// see ws.Broadcaster.Handle):
//
//   - HIERARCHICAL ("p/r/w"), from the workspace-scoped route: the workspace is
//     the third segment, resolved via scopedWorkspaceRows exactly as before
//     this step, and the snapshot is keyed by that workspace id — matching
//     what editor's handlers key their engine calls by on that mount
//     (handlers.Handlers.lspOwnerID).
//   - BARE (a single segment), from /v0/chats/:chatId/lsp/ws: that route binds
//     none of those three, so ws.clientScope falls back to the bare chat id.
//     Unlike gitSnapshot, the diagnostics are NOT re-keyed to the resolved
//     workspace: editor/LSP is spec §4.2's OWNED bucket, so the chat's own
//     REST calls (didOpen etc.) already keyed its session by the chat id
//     itself, and worktree.Resolve here exists only to confirm the chat
//     actually has a worktree to have opened a session against — a chat with
//     none replays nothing, the same degradation an unresolvable workspace
//     scope takes.
func lspSnapshot(
	appContainer *app.Container,
	engContainer *engine.Container,
) func(scope string) []lspdomain.DiagnosticsEvent {
	if engContainer == nil || engContainer.LSP == nil {
		return nil
	}
	return func(scope string) []lspdomain.DiagnosticsEvent {
		ctx := context.Background()
		if chatID, ok := bareChatScope(scope); ok {
			return chatLSPSnapshot(ctx, appContainer, engContainer, chatID)
		}
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

// chatLSPSnapshot replays chatID's own diagnostics, confirming first that the
// chat resolves to a worktree at all (spec §3) — the same existence check
// resolveChatWorktree runs for the REST routes, so a chat with no worktree
// anywhere in its ancestry degrades to no replay rather than a lookup against
// a session key nothing could ever have opened.
func chatLSPSnapshot(
	ctx context.Context,
	appContainer *app.Container,
	engContainer *engine.Container,
	chatID string,
) []lspdomain.DiagnosticsEvent {
	if appContainer == nil || appContainer.Usecases == nil || appContainer.Usecases.Worktree == nil {
		return nil
	}
	if _, err := appContainer.Usecases.Worktree.Resolve(ctx, chatID); err != nil {
		return nil
	}
	return appendDiagnostics(engContainer, make([]lspdomain.DiagnosticsEvent, 0, 1), chatID)
}

func appendDiagnostics(
	engContainer *engine.Container,
	events []lspdomain.DiagnosticsEvent,
	ownerID string,
) []lspdomain.DiagnosticsEvent {
	diags := engContainer.LSP.DiagnosticsSnapshot(ownerID)
	if len(diags) == 0 {
		return events
	}
	return append(events, lspdomain.DiagnosticsEvent{WsID: ownerID, Diagnostics: diags})
}

// terminalsSnapshot builds the Terminal-session snapshot-on-subscribe source
// (03 §1a) from the in-memory engine registry (D6: terminals are ephemeral, no
// view.db). It emits the sessions owned by the SUBSCRIBING CHAT with their real
// state (active|detached|suspended). It is empty until that chat creates a
// session.
func terminalsSnapshot(
	_ *app.Container,
	engContainer *engine.Container,
) func(scope string) []dto.TerminalSessionDTO {
	if engContainer == nil || engContainer.Terminal == nil {
		return nil
	}
	return func(scope string) []dto.TerminalSessionDTO {
		// Terminals are chat-scoped: on the flat /v0/chats/:chatId route the
		// client's scope IS the bare chat id (see ws.clientScope). List only
		// that chat's sessions — never enumerate the whole registry (the scope
		// arg exists precisely to avoid that global scan), and never a sibling
		// chat's, even one sharing this chat's worktree.
		chatID := scope
		if chatID == "" {
			return nil
		}
		ids := engContainer.Terminal.ListSessionsForChat(chatID)
		now := time.Now().UTC()
		out := make([]dto.TerminalSessionDTO, 0, len(ids))
		for _, id := range ids {
			state, ok := engContainer.Terminal.StateOf(id)
			if !ok {
				// session vanished between List and StateOf; skip
				continue
			}
			out = append(out, dto.TerminalSessionDTOFrom(id, chatID, "", state, now))
		}
		return out
	}
}
