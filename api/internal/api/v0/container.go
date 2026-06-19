package v0

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the seven realtime topics plus REST
// routes. It implements hub.Subscriber so app-layer broadcasts reach connected
// clients.
//
// The push-only Broadcaster[T] instances held here are workspaces, git, files,
// and lsp. The Terminal topic is intentionally NOT a Broadcaster[T]: PTY streams
// are bidirectional, so the Terminal topic is served by the engine.Attach
// WebSocket handler (endpoints/terminal/handlers/ws.go), whose ring-buffer
// replay is its snapshot-on-subscribe (03 §1a). It is wired separately in
// router.go.
//
// The chat WebSocket surface (chats + chatStream) has been removed per D11; the
// chat domain, repo CRUD, and usecase remain dormant TODO.
type Container struct {
	projects   *ws.Broadcaster[dto.ProjectDTO]
	repos      *ws.Broadcaster[dto.RepoDTO]
	workspaces *ws.Broadcaster[dto.WorkspaceDTO]
	threads    *ws.Broadcaster[dto.ThreadDTO]
	terminals  *ws.Broadcaster[dto.TerminalSessionDTO]
	git        *ws.Broadcaster[gitdomain.GitStatusEvent]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	app        *app.Container
	eng        *engine.Container
}

var _ hub.Subscriber = (*Container)(nil)

// New builds the v0 container and registers it as a hub subscriber.
//
// The lazy WS resource lifecycles (03 §6) are owned by the app-layer realtime
// service: the Files∪Git topics' OnSubscribe/OnUnsubscribe hooks drive the
// per-workspace file watcher via AcquireWatcher/ReleaseWatcher, and the LSP
// topic's hooks drive the per-workspace LSP host via AcquireLSP/ReleaseLSP. This
// container only wires the subscription triggers (transport); the watcher and
// LSP goroutines, the working-tree sync command, and the hub fan-out all live
// in app, and app.Container.Close tears every live resource down on shutdown.
func New(
	appContainer *app.Container,
	engContainer *engine.Container,
) *Container {
	if appContainer == nil {
		panic("v0: appContainer is required")
	}
	c := &Container{
		projects:   ws.NewBroadcaster(projectsDef(appContainer)),
		repos:      ws.NewBroadcaster(reposDef(appContainer)),
		workspaces: ws.NewBroadcaster(withProviderPollLifecycle(workspacesDef(appContainer), appContainer)),
		threads:    ws.NewBroadcaster(threadsDef(appContainer)),
		terminals:  ws.NewBroadcaster(terminalsDef(appContainer, engContainer)),
		git:        ws.NewBroadcaster(withWatcherLifecycle(gitDef(appContainer), appContainer)),
		files:      ws.NewBroadcaster(withWatcherLifecycle(filesDef(), appContainer)),
		lsp:        ws.NewBroadcaster(withLSPLifecycle(lspDef(appContainer, engContainer), appContainer)),
		app:        appContainer,
		eng:        engContainer,
	}
	if engContainer != nil && engContainer.LSP != nil {
		engContainer.LSP.OnDiagnostics(c.lsp.Push)
	}
	if engContainer != nil && engContainer.Terminal != nil {
		engContainer.Terminal.OnSessionEnded(c.onTerminalEnded)
	}
	appContainer.Hub.Register(c)
	return c
}

// onTerminalEnded emits an "ended" lifecycle frame when a PTY exits on its own
// (the reap path in the terminal engine). The handler-driven Kill path also
// pushes an "ended" frame; the broadcaster's idempotent full-replace makes the
// duplicate harmless. The owning project/repo are resolved from the workspace
// repo so the frame namespaces under projectId/repoId/wsId.
func (c *Container) onTerminalEnded(
	workspaceID string,
	sessionID string,
) {
	projectID, repoID := c.resolveWorkspaceScope(workspaceID)
	endedAt := time.Now().UTC()
	ended := dto.TerminalSessionDTOFrom(
		sessionID,
		workspaceID,
		projectID,
		repoID,
		"",
		"ended",
		endedAt,
	)
	ended.EndedAt = &endedAt
	c.terminals.Push(ended)
}

// resolveWorkspaceScope returns the project and repo ids owning workspaceID,
// or empty strings when the workspace cannot be resolved.
func (c *Container) resolveWorkspaceScope(
	workspaceID string,
) (string, string) {
	if c.app == nil {
		return "", ""
	}
	ws, err := c.app.Repositories.Workspace.Get(context.Background(), workspaceID)
	if err != nil {
		return "", ""
	}
	return ws.ProjectID, ws.RepoID
}

// withWatcherLifecycle attaches the Files∪Git watcher subscription triggers to a
// StreamDef, scoping the refcount by wsId resolved from the path or query and
// delegating to the app-layer realtime service.
func withWatcherLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireWatcher
	def.OnUnsubscribe = appContainer.Realtime.ReleaseWatcher
	return def
}

// withLSPLifecycle attaches the independent LSP subscription triggers to a
// StreamDef, scoping the refcount by wsId resolved from the path or query and
// delegating to the app-layer realtime service.
func withLSPLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireLSP
	def.OnUnsubscribe = appContainer.Realtime.ReleaseLSP
	return def
}

// withProviderPollLifecycle attaches the per-active-WS-connection provider-poll
// subscription triggers to a StreamDef, scoping the refcount by wsId resolved
// from the path or query and delegating to the app-layer realtime service
// (D10/§11). Only the single-workspace (:wsId) subscription carries a wsId; the
// workspace list scope (.../workspaces, no :wsId) resolves to "" and the
// manager no-ops, so the poll starts only when a client watches one workspace.
func withProviderPollLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireProviderPoll
	def.OnUnsubscribe = appContainer.Realtime.ReleaseProviderPoll
	return def
}

// scopeWsID resolves the workspace id from the path param, falling back to the
// query param, mirroring the dual-served Git/Files/LSP routes (T15).
func scopeWsID(
	c *gin.Context,
) string {
	if id := c.Param("wsId"); id != "" {
		return id
	}
	return c.Query("wsId")
}

// PushProject implements hub.Subscriber.
func (c *Container) PushProject(
	p dto.ProjectDTO,
) {
	c.projects.Push(p)
}

// PushRepo implements hub.Subscriber.
func (c *Container) PushRepo(
	r dto.RepoDTO,
) {
	c.repos.Push(r)
}

// PushWorkspace implements hub.Subscriber.
func (c *Container) PushWorkspace(
	w dto.WorkspaceDTO,
) {
	c.workspaces.Push(w)
}

// PushThread implements hub.Subscriber.
func (c *Container) PushThread(
	t dto.ThreadDTO,
) {
	c.threads.Push(t)
}

// PushTerminalSession implements hub.Subscriber.
func (c *Container) PushTerminalSession(
	s dto.TerminalSessionDTO,
) {
	c.terminals.Push(s)
}

// PushGit implements hub.Subscriber. It wraps the status in a wsId-carrying
// event so the Git broadcaster can scope the fan-out to a single workspace.
func (c *Container) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	// A clean tree carries a nil Files slice; normalise so the frame
	// serialises with files: [] (never null), matching the REST DTO.
	if status.Files == nil {
		status.Files = []gitdomain.GitFile{}
	}
	c.git.Push(gitdomain.GitStatusEvent{WsID: wsID, Status: status})
}

// PushFile implements hub.Subscriber.
func (c *Container) PushFile(
	evt domain.FileChangeEvent,
) {
	c.files.Push(evt)
}

// projectsDef serves the Projects topic. Its hierarchical namespace is the bare
// project id (spec §5). The snapshot returns every project as a wire DTO from
// the GORM store; the per-client prefix predicate filters it (spec §9).
func projectsDef(
	appContainer *app.Container,
) ws.StreamDef[dto.ProjectDTO] {
	return ws.StreamDef[dto.ProjectDTO]{
		Namespace: func(d dto.ProjectDTO) string { return d.ID },
		Serialize: func(d dto.ProjectDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  projectSnapshot(appContainer),
	}
}

// reposDef serves the Repos topic. Its hierarchical namespace is projectID/ID,
// so a project-scoped subscription ("p") receives every child repo (spec §5).
// The snapshot is project-scoped from the client's subscription prefix and reads
// the repos under that project from the GORM store (spec §9).
func reposDef(
	appContainer *app.Container,
) ws.StreamDef[dto.RepoDTO] {
	return ws.StreamDef[dto.RepoDTO]{
		Namespace: func(d dto.RepoDTO) string { return d.ProjectID + "/" + d.ID },
		Serialize: func(d dto.RepoDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  repoSnapshot(appContainer),
	}
}

// workspacesDef serves the Workspaces topic. Its hierarchical namespace is
// projectID/repoID/ID, so a repo-scoped subscription ("p/r") receives every
// child workspace (spec §5). The snapshot is repo-scoped from the client's
// subscription prefix and carries the merge-eligibility overlay (spec §9/§10).
func workspacesDef(
	appContainer *app.Container,
) ws.StreamDef[dto.WorkspaceDTO] {
	return ws.StreamDef[dto.WorkspaceDTO]{
		Namespace: func(d dto.WorkspaceDTO) string {
			return d.ProjectID + "/" + d.RepoID + "/" + d.ID
		},
		Serialize: func(d dto.WorkspaceDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  workspacesSnapshot(appContainer),
	}
}

// threadsDef serves the Threads topic. Its hierarchical namespace is
// projectID/repoID/workspaceID/ID, so a workspace-scoped subscription ("p/r/w")
// receives every thread in that workspace (spec §5); the per-client
// projectId/repoId/wsId Filters mirror the dual-served route's path params so
// path-first filter resolution scopes correctly. The snapshot is workspace-
// scoped from the client's subscription prefix and reads the workspace's threads
// from the global ReviewThread aggregate (W9: per-workspace thread storage is
// deferred — the aggregate stays on the global review_thread.db).
func threadsDef(
	appContainer *app.Container,
) ws.StreamDef[dto.ThreadDTO] {
	return ws.StreamDef[dto.ThreadDTO]{
		Namespace: func(d dto.ThreadDTO) string {
			return d.ProjectID + "/" + d.RepoID + "/" + d.WorkspaceID + "/" + d.ID
		},
		Serialize: func(d dto.ThreadDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  threadsSnapshot(appContainer),
		Filters: []ws.FilterDef[dto.ThreadDTO]{
			{Param: "projectId", Extract: func(d dto.ThreadDTO) string { return d.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(d dto.ThreadDTO) string { return d.RepoID }, Match: ws.ExactMatch},
			{Param: "wsId", Extract: func(d dto.ThreadDTO) string { return d.WorkspaceID }, Match: ws.ExactMatch},
		},
	}
}

// terminalsDef serves the Terminal-session lifecycle topic. Its hierarchical
// namespace is projectID/repoID/workspaceID, so a workspace-scoped subscription
// ("p/r/w") receives every session in that workspace (spec §5); the per-client
// projectId/repoId/wsId Filters mirror the dual-served route's path params so
// path-first filter resolution scopes correctly. The snapshot derives from the
// in-memory engine registry (D6: no terminal_sessions view.db). The raw PTY byte
// stream is a separate, non-broadcast WebSocket.
func terminalsDef(
	appContainer *app.Container,
	engContainer *engine.Container,
) ws.StreamDef[dto.TerminalSessionDTO] {
	return ws.StreamDef[dto.TerminalSessionDTO]{
		Namespace: func(d dto.TerminalSessionDTO) string {
			return d.ProjectID + "/" + d.RepoID + "/" + d.WorkspaceID
		},
		Serialize: func(d dto.TerminalSessionDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  terminalsSnapshot(appContainer, engContainer),
		Filters: []ws.FilterDef[dto.TerminalSessionDTO]{
			{Param: "projectId", Extract: func(d dto.TerminalSessionDTO) string { return d.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(d dto.TerminalSessionDTO) string { return d.RepoID }, Match: ws.ExactMatch},
			{Param: "wsId", Extract: func(d dto.TerminalSessionDTO) string { return d.WorkspaceID }, Match: ws.ExactMatch},
		},
	}
}

// gitDef scopes the Git topic to a single workspace by wsId. The wsId resolves
// from the PATH param on the dual-served .../workspaces/:wsId/git/status route
// (the dedicated /ws/git route was removed in W7-2). The wire
// payload is a bare GitStatus (the embedded Status), matching the REST snapshot
// of the dual-serve route; only the WsID is used for filtering, never serialized
// onto the Git stream.
func gitDef(
	appContainer *app.Container,
) ws.StreamDef[gitdomain.GitStatusEvent] {
	return ws.StreamDef[gitdomain.GitStatusEvent]{
		Namespace:     func(e gitdomain.GitStatusEvent) string { return e.WsID },
		Serialize:     func(e gitdomain.GitStatusEvent) ([]byte, error) { return json.Marshal(e.Status) },
		Snapshot:      gitSnapshot(appContainer),
		FlatNamespace: true,
		Filters: []ws.FilterDef[gitdomain.GitStatusEvent]{
			{Param: "wsId", Extract: func(e gitdomain.GitStatusEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

func filesDef() ws.StreamDef[domain.FileChangeEvent] {
	return ws.StreamDef[domain.FileChangeEvent]{
		Namespace:     func(e domain.FileChangeEvent) string { return e.WsID },
		Serialize:     func(e domain.FileChangeEvent) ([]byte, error) { return json.Marshal(e) },
		FlatNamespace: true,
		Filters: []ws.FilterDef[domain.FileChangeEvent]{
			{Param: "wsId", Extract: func(e domain.FileChangeEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

func lspDef(
	appContainer *app.Container,
	engContainer *engine.Container,
) ws.StreamDef[lspdomain.DiagnosticsEvent] {
	return ws.StreamDef[lspdomain.DiagnosticsEvent]{
		Namespace:     func(e lspdomain.DiagnosticsEvent) string { return e.WsID },
		Serialize:     func(e lspdomain.DiagnosticsEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:      lspSnapshot(appContainer, engContainer),
		FlatNamespace: true,
		Filters: []ws.FilterDef[lspdomain.DiagnosticsEvent]{
			{Param: "wsId", Extract: func(e lspdomain.DiagnosticsEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

var _ hub.Subscriber = (*Container)(nil)
