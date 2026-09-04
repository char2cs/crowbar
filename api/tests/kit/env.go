//go:build integration

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// LSPDiagnostic is a simplified diagnostic for use in integration tests, avoiding
// direct dependency on lspdomain in test callers.
type LSPDiagnostic struct {
	Message string
}

// Env is the full integration test environment: the real adapter+app+api stack
// served over a Unix domain socket (faithful to the production daemon transport,
// spec §5), backed by real SQLite stores under a WithHomeDir-isolated home, plus
// the app and engine containers for direct usecase calls.
//
// All WS helpers use deadline-based reads (no fixed-delay waits).
type Env struct {
	// URL is the fixed pseudo base URL of the test server ("http://crowbar"). The
	// host is a placeholder: every request is routed to the Unix socket by the
	// env's client/dialer, never by DNS/TCP.
	URL string
	// app exposes the application layer for direct usecase and repository calls.
	app *app.Container
	// engine exposes the engine layer for direct engine calls.
	engine *engine.Container
	// v0c exposes test helpers for the v0 API container (WaitRegistered / PushLSP).
	v0c      *v0.Container
	homeDir  string
	adapters *adapter.Container

	// server + listener serve the real router over socketPath; client + dialer
	// reach it (host-agnostic, unix-socket dial). closeOnce guards the three
	// teardown variants (Close / CloseCrashing / CloseWithoutKilling) so exactly
	// one teardown body ever runs, whichever variant (or the BuildEnvAt cleanup)
	// fires first.
	server     *http.Server
	listener   net.Listener
	socketPath string
	client     *http.Client
	dialer     *websocket.Dialer
	closeOnce  sync.Once
}

// BuildEnv spins up the full server stack (engine → adapter → app → api),
// serves it over a Unix domain socket, and registers graceful cleanup.
// Each call returns a fully isolated environment backed by its own temp home.
func BuildEnv(
	t *testing.T,
) *Env {
	t.Helper()
	return BuildEnvAt(t, tempHome(t))
}

// TempHomeForTest creates an isolated home directory with the same tolerant
// teardown semantics as BuildEnv uses internally. Call this when you need a
// homeDir to pass to BuildEnvAt for crash-recovery / restart tests.
func TempHomeForTest(t *testing.T) string {
	t.Helper()
	return tempHome(t)
}

// tempHome creates an isolated home directory for an Env with a TOLERANT
// teardown. It deliberately does NOT use t.TempDir(): a detached good-path-async
// goroutine (runAsync runs on context.WithoutCancel and the adapter registry
// lazily re-opens a per-entity DB on access) can write a per-workspace storages
// file a hair after the adapter is closed — racing t.TempDir's RemoveAll and
// flaking the run with "directory not empty" even though every assertion passed.
// This is a benign teardown race (the leaked dir is under the OS temp root and
// is reaped by the OS); we remove it best-effort with a bounded, cooperative
// retry and never fail the test on a cleanup error.
func tempHome(
	t *testing.T,
) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kitenv-")
	require.NoError(t, err, "kit: create temp home")
	t.Cleanup(func() {
		for i := 0; i < 50; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			runtime.Gosched()
		}
		// Best-effort: a residual handle from a detached write may still hold a
		// file; the OS reaps the temp tree. Do not fail the test on cleanup.
		_ = os.RemoveAll(dir)
	})
	return dir
}

// BuildEnvAt spins up the full server stack (engine → adapter → app → api)
// using a caller-supplied homeDir instead of a fresh temp directory. This
// allows two successive calls to share the same SQLite database, which is
// required for crash-recovery tests where a second "server" must read rows
// written by the first. It registers a graceful-Close cleanup; the teardown is
// idempotent, so calling a Close variant explicitly and letting cleanup run
// again is safe.
func BuildEnvAt(
	t *testing.T,
	homeDir string,
) *Env {
	t.Helper()
	env, err := NewEnvWithHome(homeDir)
	require.NoError(t, err, "kit: build env")
	// A single graceful teardown, registered so it runs BEFORE tempHome's
	// RemoveAll (cleanups are LIFO; tempHome registered its RemoveAll earlier and
	// therefore runs after). Guarded by closeOnce, so an explicit Close variant
	// wins and this cleanup is a no-op.
	t.Cleanup(func() { env.Close(t) })
	return env
}

// sockCounter uniquifies per-Env socket paths within a process so a restart
// (a second Env over the same home) binds its OWN short socket rather than
// contending on the first env's path.
var sockCounter atomic.Uint64

// kitSocketPath returns a short, unique Unix socket path in the OS temp dir.
// macOS caps sun_path at 104 bytes, so it MUST NOT live under the (long) temp
// home; a pid+counter name in $TMPDIR stays well under the limit and is unique
// across restarts and parallel test binaries.
func kitSocketPath() string {
	return filepath.Join(
		os.TempDir(),
		fmt.Sprintf("cbk-%d-%d.sock", os.Getpid(), sockCounter.Add(1)),
	)
}

// NewEnvWithHome is the restart primitive: it wires and serves the real
// adapter+app+api stack over a fresh Unix socket, rooting all on-disk state at
// homeDir (WithHomeDir isolation), and returns the Env. It takes no *testing.T
// and registers no cleanup — the caller owns teardown via a Close variant. A
// second call with the same homeDir is a "restart": it reopens the same durable
// SQLite read models the first env left behind.
func NewEnvWithHome(
	homeDir string,
) (*Env, error) {
	ctx := context.Background()

	eng, err := engine.New(ctx, engine.WithHomeDir(homeDir))
	if err != nil {
		return nil, fmt.Errorf("kit: engine: %w", err)
	}

	adapters, err := adapter.New(adapter.WithHomeDir(homeDir))
	if err != nil {
		eng.Close()
		return nil, fmt.Errorf("kit: adapter: %w", err)
	}

	appContainer, err := app.New(ctx, eng, adapters)
	if err != nil {
		eng.Close()
		_ = adapters.Close()
		return nil, fmt.Errorf("kit: app: %w", err)
	}

	router := gin.New()
	apiContainer := v0.New(appContainer, eng)
	apiContainer.Register(router.Group("/v0"))

	socketPath := kitSocketPath()
	listener, err := gateway.New("unix://" + socketPath)
	if err != nil {
		appContainer.Close()
		eng.Close()
		_ = adapters.Close()
		return nil, fmt.Errorf("kit: gateway: %w", err)
	}

	server := &http.Server{Handler: router, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()

	// Both the HTTP client and the WS dialer are host-agnostic: NetDial/DialContext
	// send every connection to the Unix socket regardless of the URL host, so the
	// existing GET/POST/DialX helpers work unchanged over the socket transport.
	dialUnix := func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}
	client := &http.Client{Transport: &http.Transport{DialContext: dialUnix}}
	dialer := &websocket.Dialer{
		NetDial:          func(_, _ string) (net.Conn, error) { return net.Dial("unix", socketPath) },
		HandshakeTimeout: 10 * time.Second,
	}

	return &Env{
		URL:        "http://crowbar",
		app:        appContainer,
		engine:     eng,
		v0c:        apiContainer,
		homeDir:    homeDir,
		adapters:   adapters,
		server:     server,
		listener:   listener,
		socketPath: socketPath,
		client:     client,
		dialer:     dialer,
	}, nil
}

// Close performs a GRACEFUL shutdown, faithful to the production daemon's
// ordered drain (spec §3.8): stop accepting + drain in-flight HTTP, drain every
// post-commit reactor and each asynx singleton (app.Shutdown), release realtime
// resources (file watchers / LSP hosts) and engine OS children, then
// WAL-checkpoint + close the SQLite DBs and remove the socket. Releasing the DB
// handles is what lets a second Env reopen the same home (restart tests). The
// teardown body runs at most once (closeOnce), so an explicit Close plus the
// BuildEnvAt cleanup is safe.
func (e *Env) Close(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.server.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.Close: server shutdown: %v", err)
		}
		if err := e.app.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.Close: app drain: %v", err)
		}
		e.app.Close()
		e.engine.Close()
		if err := e.adapters.Close(); err != nil {
			t.Logf("kit.Env.Close: adapter: %v", err)
		}
		_ = e.listener.Close()
	})
}

// CloseCrashing simulates a CRASH (SIGKILL): it releases resources WITHOUT the
// graceful drain — no server.Shutdown, no app.Shutdown — so post-commit reactors
// and in-flight asynx work are abandoned mid-flight (e.g. a delete cascade killed
// before it purges, a half-provisioned worktree). It still closes the SQLite DBs
// and OS handles so a second Env can reopen the same home and drive recovery.
// Shares closeOnce with the other variants (call exactly one).
func (e *Env) CloseCrashing(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		_ = e.server.Close() // abrupt: drop the listener + active conns, no drain.
		e.app.Close()        // release realtime FDs (watchers/LSP) — not a data drain.
		e.engine.Close()
		if err := e.adapters.Close(); err != nil {
			t.Logf("kit.Env.CloseCrashing: adapter: %v", err)
		}
		_ = e.listener.Close()
	})
}

// CloseWithoutKilling tears the env down gracefully (drain + close server, app,
// and DBs) but LEAVES the engine's OS child processes running — the PTY shells
// and LSP servers are not killed (no engine.Close). Use it for tests that assert
// on processes surviving a daemon teardown. Shares closeOnce with the other
// variants (call exactly one). The OS reaps the leaked children when the test
// binary exits.
func (e *Env) CloseWithoutKilling(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.server.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.CloseWithoutKilling: server shutdown: %v", err)
		}
		if err := e.app.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.CloseWithoutKilling: app drain: %v", err)
		}
		e.app.Close()
		// Deliberately NOT e.engine.Close(): leave PTY/LSP children running.
		if err := e.adapters.Close(); err != nil {
			t.Logf("kit.Env.CloseWithoutKilling: adapter: %v", err)
		}
		_ = e.listener.Close()
	})
}

// SocketPath returns the Unix domain socket path this Env serves on.
func (e *Env) SocketPath() string { return e.socketPath }

// HomeDir returns the home directory path used by this Env.
func (e *Env) HomeDir() string { return e.homeDir }

// ShutdownTerminal calls Shutdown on the terminal engine, flushing all live
// sessions to disk and persisting their metadata with state="suspended". Call
// this before Close when simulating a daemon restart in tests: Shutdown ensures
// scrollback is durable so the next BuildEnvAt can restore sessions via
// RestorePersistedSessions.
func (e *Env) ShutdownTerminal() {
	if e.engine != nil && e.engine.Terminal != nil {
		e.engine.Terminal.Shutdown()
	}
}

// WorktreePath returns the on-disk git worktree directory for a workspace by
// reading the server's own persisted path from the durable read model
// (domain.Workspace.WorktreePath). Task 3b retired the UUID worktree layout in
// favour of the human-readable <home>/projects/<project>/<slug>/<branch> (spec
// §3.9); rather than re-derive the slug — which the kit cannot reach, since
// worktreepath is doubly-internal — it returns the ground-truth path the
// provisioner used for `git worktree add`, so a managed child, an adopted main
// (repo.Path), or a .home leaf all resolve correctly. worktreePath is
// server-side only and never surfaced in the WorkspaceDTO (spec §8/§5). The
// projectID/repoID params are retained for call-site stability; the path is
// keyed solely by wsID.
func (e *Env) WorktreePath(
	projectID string,
	repoID string,
	wsID string,
) string {
	_ = projectID
	_ = repoID
	ws, err := e.app.Repositories.Workspace.Get(context.Background(), wsID)
	if err != nil {
		panic(fmt.Sprintf("kit: WorktreePath: get workspace %q: %v", wsID, err))
	}
	return ws.WorktreePath
}

// WorkspaceStorageDir mirrors worktreepath.StorageDir:
// <home>/projects/<P>/<R>/workspaces/<W>/storages.
func (e *Env) WorkspaceStorageDir(
	projectID string,
	repoID string,
	wsID string,
) string {
	return filepath.Join(
		e.homeDir,
		"projects",
		projectID,
		repoID,
		"workspaces",
		wsID,
		"storages",
	)
}

// PushLSP injects a batch of diagnostics into the LSP broadcaster for wsID,
// bypassing the engine OnDiagnostics callback. Use in integration tests when no
// real language server is attached.
func (e *Env) PushLSP(
	wsID string,
	diags []LSPDiagnostic,
) {
	out := make([]lspdomain.Diagnostic, len(diags))
	for i, d := range diags {
		out[i] = lspdomain.Diagnostic{Message: d.Message}
	}
	e.v0c.PushLSP(lspdomain.DiagnosticsEvent{
		WsID:        wsID,
		Diagnostics: out,
	})
}

// FileEvent is a simplified file change event for use in integration tests,
// avoiding direct dependency on domain in test callers.
type FileEvent struct {
	WsID    string
	Type    string
	Path    string
	NewPath string
}

// PushFile injects a FileChangeEvent directly into the files broadcaster,
// bypassing the OS watcher. Use in integration tests when deterministic event
// injection is required without a running file watcher.
func (e *Env) PushFile(
	evt FileEvent,
) {
	e.v0c.PushFile(domain.FileChangeEvent{
		WsID:    evt.WsID,
		Type:    domain.FileChangeType(evt.Type),
		Path:    evt.Path,
		NewPath: evt.NewPath,
	})
}

// GitFileEntry is a simplified git file status for use in integration tests,
// avoiding direct dependency on gitdomain in test callers.
type GitFileEntry struct {
	Path   string
	Status string
	Staged bool
}

// GitStatusEvent is a simplified git status event for use in integration tests,
// avoiding direct dependency on gitdomain in test callers.
type GitStatusEvent struct {
	WsID   string
	Branch string
	Files  []GitFileEntry
}

// ProviderState is a scripted provider poll result for the mock-provider seam
// (PushProviderState). It mirrors the fields the workspace aggregate consumes to
// drive PR-status and protected-branch transitions, avoiding a direct dependency
// on the workspace repository's ProviderInput in test callers.
type ProviderState struct {
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// PushGit injects a GitStatus directly into the git broadcaster,
// bypassing the file watcher and git write usecases. Use in integration tests
// when deterministic git status injection is required.
func (e *Env) PushGit(
	evt GitStatusEvent,
) {
	files := make([]gitdomain.GitFile, len(evt.Files))
	for i, f := range evt.Files {
		files[i] = gitdomain.GitFile{
			Path:   f.Path,
			Status: gitdomain.GitFileStatus(f.Status),
			Staged: f.Staged,
		}
	}
	e.v0c.PushGit(evt.WsID, gitdomain.GitStatus{
		WsID:   evt.WsID,
		Branch: evt.Branch,
		Files:  files,
	})
}

// DialProjects opens a WS watcher on the list-scoped Projects stream
// (WS /v0/projects) and blocks until the server has registered this client. A
// list-scope subscriber receives every project's ProjectDTO (snapshot + live).
func (e *Env) DialProjects(
	t *testing.T,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/projects"))
	e.v0c.WaitNProjectsRegistered(1)
	return w
}

// DialRepos opens a WS watcher on the project-scoped Repos stream
// (WS /v0/projects/:projectId/repos) and blocks until registration. The
// subscriber receives that project's RepoDTOs (snapshot + live).
func (e *Env) DialRepos(
	t *testing.T,
	projectID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/projects/"+projectID+"/repos"))
	e.v0c.WaitNReposRegistered(1)
	return w
}

// DialRepoChats opens a WS watcher on the repo-scoped agent-chat stream
// (WS /v0/projects/:projectId/repos/:repoId/chats/ws) and blocks until
// registration. It is the repo-wide replacement for the deleted `workspaces`
// stream: every worktree in the repo still reports here, now as a
// worktree_state frame keyed by the chat that holds it (spec §7.4).
func (e *Env) DialRepoChats(
	t *testing.T,
	projectID string,
	repoID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/projects/"+projectID+"/repos/"+repoID+"/chats/ws"))
	e.v0c.WaitNAgentChatsRegistered(1)
	return w
}

// DialChat opens a WS watcher on the per-chat lifecycle stream
// (WS /v0/chats/:chatId/ws) and blocks until registration.
//
// This is the chat-scoped replacement for watching ONE workspace's stream
// (router.go says so at the mount): it resolves the chat's worktree through
// resolveChatWorktree, which is what starts the daemon's provider poll — the
// same side effect the old exact-workspace subscription carried.
func (e *Env) DialChat(
	t *testing.T,
	chatID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/ws"))
	e.v0c.WaitNAgentChatsRegistered(1)
	return w
}

// DialThreads opens a WS watcher on the workspace-scoped Threads stream and
// blocks until registration. The subscriber receives that workspace's
// ThreadDTOs (snapshot + live).
func (e *Env) DialThreads(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, e.wsScope(projectID, repoID, wsID)+"/threads"))
	e.v0c.WaitNThreadsRegistered(1)
	return w
}

// DialTerminals opens a WS watcher on the CHAT-scoped Terminals lifecycle
// stream and blocks until registration. The subscriber receives the
// TerminalSessionDTOs of the sessions that chat OWNS (snapshot + live) — never
// a sibling chat's, even one sharing the same worktree. This is the lifecycle
// topic, NOT the raw PTY stream (.../terminals/:sessionId/ws).
func (e *Env) DialTerminals(
	t *testing.T,
	chatID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/terminals"))
	e.v0c.WaitNTerminalsRegistered(1)
	return w
}

// DialTerminalPTY opens a WS watcher on the raw PTY stream co-located at
// /v0/chats/:chatId/terminals/:sessionId/ws (W7-2). This is NOT a broadcaster
// topic (it's a direct engine pipe), so there is no WaitNRegistered gate; the
// PTY emits its first frame promptly on connect.
func (e *Env) DialTerminalPTY(
	t *testing.T,
	chatID string,
	sessionID string,
) *WSWatcher {
	t.Helper()
	return Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/terminals/"+sessionID+"/ws"))
}

// DialGit opens a WS watcher on the CHAT-scoped, co-located Git status stream
// (/v0/chats/:chatId/git/status) and blocks until registration. The chat names
// the worktree whose status is served, so every chat holding that worktree sees
// the same state — the old /workspaces/:wsId mount is gone (spec §8 step 6).
func (e *Env) DialGit(
	t *testing.T,
	chatID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/git/status"))
	e.v0c.WaitNGitRegistered(1)
	return w
}

// DialFiles opens a WS watcher on the CHAT-scoped, co-located Files stream
// (/v0/chats/:chatId/files/ws, change-only, no snapshot) and blocks until
// registration.
func (e *Env) DialFiles(
	t *testing.T,
	chatID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/files/ws"))
	e.v0c.WaitNFilesRegistered(1)
	return w
}

// DialLSP opens a WS watcher on the CHAT-scoped, co-located LSP stream
// (/v0/chats/:chatId/lsp/ws) and blocks until registration.
func (e *Env) DialLSP(
	t *testing.T,
	chatID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/lsp/ws"))
	e.v0c.WaitNLSPRegistered(1)
	return w
}

// wsScope builds the hierarchical workspace path prefix. Threads is the ONLY
// surviving "/workspaces/:wsId/..." path off the repo-scoped group (router.go);
// every other leaf that once hung here is addressed through chatScope now.
func (e *Env) wsScope(
	projectID string,
	repoID string,
	wsID string,
) string {
	return "/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces/" + wsID
}

// chatScope builds the FLAT chat path prefix shared by all chat-scoped routes.
// Chat ids are globally unique, so nothing below it re-nests under a
// project/repo pair.
func (e *Env) chatScope(
	chatID string,
) string {
	return "/v0/chats/" + chatID
}

// OwningChatID returns the id of the chat row that OWNS the given workspace —
// the chat every chat-scoped route addresses that workspace's worktree through.
//
// It first makes sure the row exists, through the daemon's own EnsureOwningChat
// (the live-path narrowing of the boot backfill, taking the same decision by
// the same code): a workspace created mid-run is otherwise owed its row only by
// the NEXT boot's backfill, and a test that just created one would find no chat
// to address it by.
//
// The read-back is in-process, not over HTTP: a workspace is not addressable
// any more (spec §8 step 6 deleted GET .../workspaces/:wsId), so there is no
// route to ask. It takes the SAME two calls the wire DTO's own resolver takes
// (repositories.Container.owningChatIDFor) — ListChatsByWorkspace then
// agentusecase.ResolveOwningChat — so the answer is still the production
// resolver's, never a second one derived here.
func (e *Env) OwningChatID(
	t *testing.T,
	wsID string,
) string {
	t.Helper()
	ctx := context.Background()
	ws, err := e.app.Repositories.Workspace.Get(ctx, wsID)
	require.NoError(t, err, "OwningChatID: read workspace %s", wsID)
	require.NoError(
		t,
		e.app.Usecases.AgentChatFolder.EnsureOwningChat(ctx, ws),
		"OwningChatID: ensure the owning chat for %s",
		wsID,
	)
	// The mint is an asynx write; drain the projections so the read below sees
	// the row rather than racing it (no polling, no sleep).
	e.Quiesce()

	rows, err := e.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, wsID)
	require.NoError(t, err, "OwningChatID: list the chats holding %s", wsID)
	owner, ok := agentusecase.ResolveOwningChat(rows)
	require.Truef(
		t,
		ok,
		"workspace %s must be held by an owning chat for the chat-scoped routes",
		wsID,
	)
	return owner.ID
}

// WorktreeFrame projects one chat-feed frame down to the flat workspace map the
// deleted `workspaces` stream used to send, so a predicate written against that
// stream reads the same keys off this one.
//
// It is an UNWRAP, not a translation. A worktree_state frame is the workspace's
// own WorkspaceDTO put through dto.ChatWorktreeFrom and hung under `worktree`
// (container.pushChatWorktree), so every key here is the byte the workspace
// stream would have sent — this only lifts them back out of the envelope and
// restores the two identity fields the envelope, not the payload, carries: `id`
// (the frame's workspaceId) and `repoId`.
//
// projectId, createdAt and kind are NOT recoverable: the chat feed never
// carried them. Nothing asserts on them off this stream.
//
// A frame that is not a worktree_state one is returned UNCHANGED, which is what
// keeps the shared WaitForState usable for the flat DTO streams (threads,
// repos, projects) as well as this one.
func WorktreeFrame(
	m map[string]any,
) map[string]any {
	wt, ok := m["worktree"].(map[string]any)
	if !ok {
		return m
	}
	wsID, _ := m["workspaceId"].(string)
	if wsID == "" {
		return m
	}
	out := make(map[string]any, len(wt)+3)
	for k, v := range wt {
		out[k] = v
	}
	out["id"] = wsID
	out["chatId"] = m["chatId"]
	if repoID, ok := m["repoId"]; ok {
		out["repoId"] = repoID
	}
	return out
}

// readUntilWorktree is ReadUntil over the chat feed with WorktreeFrame applied
// to every frame before the predicate sees it, returning the PROJECTED map.
func readUntilWorktree(
	t *testing.T,
	w *WSWatcher,
	timeout time.Duration,
	pred func(map[string]any) bool,
) map[string]any {
	t.Helper()
	var hit map[string]any
	w.ReadUntil(t, timeout, func(raw map[string]any) bool {
		m := WorktreeFrame(raw)
		if pred(m) {
			hit = m
			return true
		}
		return false
	})
	return hit
}

// WaitForWorkspace reads WS events from w until a workspace with the given ID
// and matching predicate is found. Returns the decoded Workspace map.
func WaitForWorkspace(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	timeout time.Duration,
	pred func(map[string]any) bool,
) map[string]any {
	t.Helper()
	return readUntilWorktree(
		t,
		w,
		timeout,
		func(msg map[string]any) bool {
			if msg["id"] != wsID {
				return false
			}
			return pred(msg)
		},
	)
}

// WaitForState reads WS DTOs from w until one with the given id reaches the
// given status, returning the decoded DTO. It is the aggregate-agnostic state
// watcher (spec §5): the same shape works for Workspace and ReviewThread DTOs,
// which both carry "id" and "status". A blank status matches the omitempty ""
// state.
func WaitForState(
	t *testing.T,
	w *WSWatcher,
	id string,
	status string,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return readUntilWorktree(
		t,
		w,
		timeout,
		func(msg map[string]any) bool {
			if msg["id"] != id {
				return false
			}
			got, _ := msg["status"].(string)
			return got == status
		},
	)
}

// WaitForListLen reads DTOs from a list-scoped stream w until it has observed n
// DISTINCT entity ids (snapshot + live merged), returning the latest DTO seen
// for each id in first-arrival order. Use it to await "the list now has n items"
// without polling; a repeated/updated DTO for an already-seen id does not
// advance the count.
func WaitForListLen(
	t *testing.T,
	w *WSWatcher,
	n int,
	timeout time.Duration,
) []map[string]any {
	t.Helper()
	if n <= 0 {
		return nil
	}
	seen := make(map[string]map[string]any, n)
	order := make([]string, 0, n)
	readUntilWorktree(
		t,
		w,
		timeout,
		func(msg map[string]any) bool {
			id, _ := msg["id"].(string)
			if id == "" {
				return false
			}
			if _, ok := seen[id]; !ok {
				order = append(order, id)
			}
			seen[id] = msg
			return len(seen) >= n
		},
	)
	out := make([]map[string]any, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

// WaitForWorkspaceState reads WS events from w until a WorkspaceDTO with the
// given id reaches the given status. Returns the decoded DTO map. A blank status
// matches the omitempty "" state (commits, no PR). It delegates to WaitForState.
func WaitForWorkspaceState(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	status string,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return WaitForState(t, w, wsID, status, timeout)
}

// WaitForWorkComplete blocks until wsID's in-flight async mutation has STARTED
// and then FINISHED, and returns the final DTO. It is the completion barrier for
// the fire-and-forget 202 handlers, whose slow work runs in a detached goroutine
// that no projection drain can see.
//
// It reads the daemon's OWN "am I busy" overlay. Every detached op is bracketed
// by the handler: BeginWork(wsID) on the request goroutine, then EndWork(wsID) on
// EVERY exit — success, error or panic — after the work function has returned.
// Both broadcast a WorkspaceDTO, so the stream carries working:true … working:false,
// and the falling edge is the daemon stating that the op is over. Waiting for the
// RISING edge first is what makes the falling one meaningful: a workspace at rest
// already reads working:false, so matching that alone would return before the op
// had even begun.
//
// This is the barrier a merge needs. A merge's status fold is NOT one: a
// conflicting child is broadcast as pr-conflicts by the merge-tree PREDICTION
// overlay before any merge is attempted, so a test keyed on that status can
// wake up while the merge is still mid-conflict and read a worktree that has not
// been aborted yet — which is precisely the race the old 2-second cleanliness
// poll was hiding.
func WaitForWorkComplete(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	isWorking := func(m map[string]any, want bool) bool {
		if m["id"] != wsID {
			return false
		}
		working, ok := m["working"].(bool)
		return ok && working == want
	}
	readUntilWorktree(t, w, timeout, func(m map[string]any) bool { return isWorking(m, true) })
	return readUntilWorktree(t, w, timeout, func(m map[string]any) bool { return isWorking(m, false) })
}

// WaitForWorkspaceLastError reads WS events from w until a WorkspaceDTO with the
// given id carries a non-empty lastError. Returns the lastError string.
func WaitForWorkspaceLastError(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	timeout time.Duration,
) string {
	t.Helper()
	msg := readUntilWorktree(
		t,
		w,
		timeout,
		func(m map[string]any) bool {
			if m["id"] != wsID {
				return false
			}
			le, _ := m["lastError"].(string)
			return le != ""
		},
	)
	le, _ := msg["lastError"].(string)
	return le
}

// GET issues a GET request to path and returns the response.
func (e *Env) GET(
	t *testing.T,
	path string,
) *http.Response {
	t.Helper()
	resp, err := e.client.Get(e.URL + path)
	require.NoError(
		t,
		err,
	)
	return resp
}

// POST issues a POST request with a JSON body to path and returns the response.
func (e *Env) POST(
	t *testing.T,
	path string,
	body any,
) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(
		t,
		err,
	)
	resp, err := e.client.Post(
		e.URL+path,
		"application/json",
		bytes.NewReader(raw),
	)
	require.NoError(
		t,
		err,
	)
	return resp
}

// PUT issues a PUT request with a JSON body.
func (e *Env) PUT(
	t *testing.T,
	path string,
	body any,
) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(
		t,
		err,
	)
	req, err := http.NewRequest(
		http.MethodPut,
		e.URL+path,
		bytes.NewReader(raw),
	)
	require.NoError(
		t,
		err,
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	require.NoError(
		t,
		err,
	)
	return resp
}

// PATCH issues a PATCH request with a JSON body.
func (e *Env) PATCH(
	t *testing.T,
	path string,
	body any,
) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(
		t,
		err,
	)
	req, err := http.NewRequest(
		http.MethodPatch,
		e.URL+path,
		bytes.NewReader(raw),
	)
	require.NoError(
		t,
		err,
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	require.NoError(
		t,
		err,
	)
	return resp
}

// DELETE issues a DELETE request.
func (e *Env) DELETE(
	t *testing.T,
	path string,
) *http.Response {
	t.Helper()
	req, err := http.NewRequest(
		http.MethodDelete,
		e.URL+path,
		nil,
	)
	require.NoError(
		t,
		err,
	)
	resp, err := e.client.Do(req)
	require.NoError(
		t,
		err,
	)
	return resp
}

// DELETEJ issues a DELETE request with a JSON body.
func (e *Env) DELETEJ(
	t *testing.T,
	path string,
	body any,
) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(
		t,
		err,
	)
	req, err := http.NewRequest(
		http.MethodDelete,
		e.URL+path,
		bytes.NewReader(raw),
	)
	require.NoError(
		t,
		err,
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	require.NoError(
		t,
		err,
	)
	return resp
}

// MutationID decodes a mutation envelope ({"success":true,"data":{"id":"..."}})
// and returns the id. It closes the response body. Use after RequireStatus.
func MutationID(
	t testing.TB,
	resp *http.Response,
) string {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), "MutationID body: %s", string(raw))
	require.NotEmpty(t, env.Data.ID, "MutationID: data.id must not be empty; body: %s", string(raw))
	return env.Data.ID
}

// DecodeEnvData decodes the data field of a query envelope
// ({"success":true,"data":{...}}) into dest. It closes the response body.
func DecodeEnvData(
	t *testing.T,
	resp *http.Response,
	dest any,
) {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), "DecodeEnvData body: %s", string(raw))
	require.NoError(t, json.Unmarshal(env.Data, dest), "DecodeEnvData.data body: %s", string(raw))
}

// RegisterProject creates a project via POST /v0/projects and returns the
// server-assigned project id. Creation is async (202 + empty body, spec §4): the
// id is learned from the ProjectDTO delivered on the Projects WS stream. The WS
// is dialled BEFORE the POST so the create broadcast is never missed.
func (e *Env) RegisterProject(
	t *testing.T,
	name string,
	path string,
) string {
	t.Helper()
	w := e.DialProjects(t)
	resp := e.POST(t, "/v0/projects", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	msg := w.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		p, _ := m["path"].(string)
		return p == path
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "RegisterProject: ProjectDTO must carry an id")
	return id
}

// RegisterRepo creates a repo via POST /v0/projects/:projectId/repos and returns
// the server-assigned repo id. Creation is async (202 + empty body, spec §4):
// the id is learned from the RepoDTO delivered on the Repos WS stream. The WS is
// dialled BEFORE the POST so the create broadcast is never missed. path may be
// empty for repos that don't need git operations.
func (e *Env) RegisterRepo(
	t *testing.T,
	projectID string,
	name string,
	path string,
) string {
	t.Helper()
	w := e.DialRepos(t, projectID)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	msg := w.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		return m["name"] == name && m["projectId"] == projectID
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "RegisterRepo: RepoDTO must carry an id")
	return id
}

// CreateWorkspace creates a worktree on a caller-named branch and returns the
// server-assigned workspace UUID.
//
// It goes through the USECASE, not HTTP, because there is no longer an HTTP way
// to ask: spec §8 step 6 deleted the whole `workspaces` endpoint group, and the
// chat-scoped surface that replaced it only forks with an auto-derived branch
// name or imports a branch that already exists. Naming the branch is a FIXTURE
// need, not a product one — a test that asserts on "feature-x" has to be able
// to say "feature-x" — so it is met at the layer that still offers it rather
// than by adding a product capability nothing ships.
//
// Usecases.Workspace.CreateChild is the same call the live import path makes
// (usecases.worktreeChildCreator.CreateImportedWorkspace), with the same
// arguments, so a fixture worktree is born exactly as a real one is — including
// the owning chat every worktree is created under. Only the caller differs.
func (e *Env) CreateWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
) string {
	t.Helper()
	return e.createWorkspace(t, projectID, repoID, branch, "")
}

// CreateChildWorkspace creates a child workspace under parentID (a WORKSPACE
// id) and returns the UUID. See CreateWorkspace for why this is a usecase call.
func (e *Env) CreateChildWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) string {
	t.Helper()
	return e.createWorkspace(t, projectID, repoID, branch, parentID)
}

func (e *Env) createWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) string {
	t.Helper()
	ctx := context.Background()
	repo, err := e.app.GORM.Repositories.FindByKey(ctx, repoID)
	require.NoError(t, err, "createWorkspace: read repo %s", repoID)
	require.NotNil(t, repo, "createWorkspace: repo %s must exist", repoID)
	// ParentBranch is the START POINT the new worktree is cut from, and it must
	// be supplied here rather than left to resolveInherited: that defaulting only
	// runs for a caller that names NEITHER RepoID nor RepoPath, and this one
	// names both (as the live import path does). Left blank, `git worktree add`
	// rev-parses "" and dies.
	parentBranch := repo.DefaultBranch
	if parentID != "" {
		parent, perr := e.app.Repositories.Workspace.Get(ctx, parentID)
		require.NoError(t, perr, "createWorkspace: read parent workspace %s", parentID)
		parentBranch = parent.Branch
	}
	// Forced true for the same reason the live import path forces it: the
	// taxonomy default is "inherit whether the parent owns a worktree", and a
	// fixture asking for a branch is asking for a real worktree on it.
	ownWorktree := true
	ws, err := e.app.Usecases.Workspace.CreateChild(ctx, wsusecase.CreateChildInput{
		RepoID:       repoID,
		ProjectID:    projectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
		OwnWorktree:  &ownWorktree,
	})
	require.NoErrorf(t, err, "createWorkspace: create %q under %q", branch, parentID)
	// The create's own writes are asynx commands; drain them so a caller that
	// reads the row (or dials its chat) next sees it rather than racing it.
	e.Quiesce()
	require.NotEmpty(t, ws.ID, "createWorkspace: the created workspace must carry an id")
	return ws.ID
}

// CreateWorkspaceWithChat is CreateWorkspace plus the id of the chat the new
// worktree is born under — the pair almost every chat-scoped test needs, in one
// call instead of a create followed by a resolve.
func (e *Env) CreateWorkspaceWithChat(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (string, string) {
	t.Helper()
	wsID := e.createWorkspace(t, projectID, repoID, branch, parentID)
	return wsID, e.OwningChatID(t, wsID)
}

// ImportedRepo bundles the ids a full project+repo import yields: the project,
// its discovered repository, and the workspace adopted from the repo's default
// (main) worktree. It is the integration-suite analogue of the package-tests
// importedRepo fixture.
//
// ChatID is the chat that OWNS WorkspaceID — the id every chat-scoped route
// addresses that worktree through. It is bundled here rather than looked up per
// call site because the import mints it: a repo import creates each adopted
// worktree under a chat (usecases.hierarchyOwningChats), so the id is already
// on the chat list this fixture reads and asking for it again would be a second
// resolution of a fact the fixture already holds.
type ImportedRepo struct {
	ProjectID   string
	RepoID      string
	WorkspaceID string
	ChatID      string
	RepoPath    string
}

// Quiesce blocks until every per-type asynx projection has drained (dispatch
// queue idle + all projection handlers run) — the deterministic read-your-writes
// barrier. The create/mutation helpers observe completion on the hub broadcast
// (WS), but the store/list read model is an INDEPENDENT async projection that can
// trail the WS frame; call Quiesce after a mutation so a subsequent read of the
// list/store is guaranteed consistent, with no polling and no timeouts.
func (e *Env) Quiesce() {
	e.app.Repositories.WaitQuiescent()
}

// QuiesceReactors is Quiesce PLUS the join of every post-commit REACTOR the
// mutation set in motion. It is the barrier for the async cascades whose whole
// effect lands outside the aggregate — above all DELETE, which converges only
// when the read-model row is gone AND the worktree is physically gone from disk.
//
// Quiesce alone is NOT enough for those, and the gap is structural rather than a
// matter of degree. A reactor is an asynx SUBSCRIBER that immediately detaches:
// its handler does `drainWG.Add(1); go run(...)` and returns. WaitPublish
// therefore observes the handler as complete the moment it has spawned the
// goroutine — while the purge it spawned has not yet Forgotten the aggregate or
// rm'd the worktree. The detached goroutine is joined by exactly one thing, the
// container's drain WaitGroup, which is the same one the daemon's graceful
// shutdown waits on. So this is the production drain, used as a test barrier.
//
// The three steps are a CHAIN, and each is needed:
//
//  1. WaitQuiescent — dispatch the command and fold its projections. This must be
//     first for two reasons: it is what gets the deleted event to the reactor, so
//     the reactor's Add(1) has happened before anything waits on the WaitGroup
//     (waiting first would find a zero counter and synchronise with nothing); and
//     it lands the tombstone in the read model, which is precisely what the
//     reactor's own gate is waiting to see.
//  2. Drain().Gate.WaitIdle(context.Background()) — join the detached purge: rm the worktree, Forget the
//     aggregate, cascade to dependents.
//  3. WaitQuiescent again — the purge's Forget is ITSELF an asynx command, and the
//     read-model row is dropped by that command's PROJECTION. So the reactor
//     finishing is not the end of the chain: without this last fold, the row the
//     delete is supposed to remove is still sitting there. (Found the hard way:
//     step 3 is exactly the assertion that failed without it.)
func (e *Env) QuiesceReactors() {
	e.app.Repositories.WaitQuiescent()
	e.app.Repositories.Drain().Gate.WaitIdle(context.Background())
	e.app.Repositories.WaitQuiescent()
}

// ImportRepo creates a real git repo at the supplied path (or inits a fresh one
// when path is empty), imports it as a project, and runs the full per-repo
// import (RegisterRepo) which adopts the default-branch worktree as a workspace.
// It returns the project/repo/adopted-workspace ids plus the repo path. The
// adopted workspace id is learned from the WorkspaceDTO broadcast on the
// repo-scoped Workspaces WS stream (dial-before-import), never from a sync body.
func (e *Env) ImportRepo(
	t *testing.T,
	name string,
	path string,
) ImportedRepo {
	t.Helper()
	if path == "" {
		path = InitRepo(t)
	}
	projectID := e.RegisterProject(t, name, path)
	// The repo import (POST .../repos) runs the full importer, which derives the
	// repo NAME from the on-disk directory — not from the request body — so the
	// RepoDTO is matched by projectId, not name. Dial repos BEFORE the POST so the
	// import broadcast is never missed.
	reposWS := e.DialRepos(t, projectID)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	repo := reposWS.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "ImportRepo: import must broadcast a RepoDTO with an id")

	// The adopted main worktree is persisted after the repo in the same async
	// import job. It is read back, not awaited on the wire, and that is forced:
	// the adoption is the one worktree event NOTHING can subscribe to in time.
	// A chat-feed frame is only reachable through a repo-scoped mount, and the
	// repo whose id that mount needs is created BY this same job — so by the
	// moment there is a repoId to dial, the adoption it would have carried has
	// already happened. Quiesce drains the projections, then the chat list is
	// read; both are real signals, neither is a poll or a sleep.
	e.Quiesce()
	chats := e.ListChats(t, projectID, repoID)
	var wsID, chatID string
	for _, c := range chats {
		wt, ok := c["worktree"].(map[string]any)
		if !ok {
			continue
		}
		if isDefault, _ := wt["isDefault"].(bool); !isDefault {
			continue
		}
		wsID, _ = c["workspaceId"].(string)
		chatID, _ = c["id"].(string)
		break
	}
	require.NotEmptyf(
		t,
		wsID,
		"ImportRepo: import must adopt the repo's default worktree; chats: %s",
		MustJSON(chats),
	)
	require.NotEmpty(t, chatID, "ImportRepo: the adopted worktree must be born under a chat")
	return ImportedRepo{
		ProjectID:   projectID,
		RepoID:      repoID,
		WorkspaceID: wsID,
		ChatID:      chatID,
		RepoPath:    path,
	}
}

// ListChats reads a repo's chat rows over the real REST surface
// (GET .../repos/:repoId/chats) — the ONE read that answers everything the
// deleted workspace list used to, since each worktree-owning row carries its
// git state inline (spec §5).
func (e *Env) ListChats(
	t *testing.T,
	projectID string,
	repoID string,
) []map[string]any {
	t.Helper()
	resp := e.GET(t, "/v0/projects/"+projectID+"/repos/"+repoID+"/chats")
	RequireStatus(t, resp, http.StatusOK)
	var chats []map[string]any
	DecodeEnvData(t, resp, &chats)
	return chats
}

// DeleteWorkspaceCascade tombstones a workspace and sets its purge cascade in
// motion — the exact call the deleted DELETE .../workspaces/:wsId route made.
//
// It is a FIXTURE primitive, for a test that needs a tombstoned workspace as a
// PRECONDITION rather than as its subject. The only HTTP way to reach a delete
// now is DELETE .../chats/:chatId, which additionally hard-purges the owning
// chat in the same request; a test about what happens to the WORKSPACE after a
// crash would then be entangled with that second teardown. This is the narrow
// call, so the precondition is the one the test actually means.
func (e *Env) DeleteWorkspaceCascade(
	t *testing.T,
	wsID string,
) {
	t.Helper()
	require.NoError(
		t,
		e.app.Usecases.Workspace.DeleteCascade(context.Background(), wsID),
		"DeleteWorkspaceCascade: %s",
		wsID,
	)
}

// WorkspaceRow returns a workspace's status in the DURABLE read model, and
// whether it still has a row there at all.
//
// It reads in-process, and that is not a shortcut — it is the only surface left.
// A workspace is addressed through the chat that owns it now, so once that chat
// is purged (which DELETE .../chats/:id does, in the same request that
// tombstones the workspace) no wire read can reach the row: the chat list would
// answer "absent" for a workspace whose row is very much still there. Asserting
// the DELETE invariant over the wire would therefore be vacuously true.
//
// It reads exactly what the BOOT ORPHAN-SWEEP lists from — the central durable
// read model, state/store/workspace.db, via ListInRepo — so a test asserting
// "the sweep reaped the residual row" is asking the same store the sweep asked,
// not a second view that could disagree.
func (e *Env) WorkspaceRow(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) (status string, present bool) {
	t.Helper()
	rows, err := e.app.Repositories.Workspace.ListInRepo(context.Background(), projectID, repoID)
	require.NoError(t, err, "WorkspaceRow: list %s/%s", projectID, repoID)
	for _, w := range rows {
		if w.ID == wsID {
			return string(w.Status), true
		}
	}
	return "", false
}

// WorktreeChats returns the repo's chat rows that OWN a worktree, projected to
// the flat workspace shape (WorktreeFrame's REST twin), keyed by workspace id.
// It is the read-model replacement for the deleted workspace list.
func (e *Env) WorktreeChats(
	t *testing.T,
	projectID string,
	repoID string,
) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, c := range e.ListChats(t, projectID, repoID) {
		wsID, _ := c["workspaceId"].(string)
		wt, ok := c["worktree"].(map[string]any)
		if wsID == "" || !ok {
			continue
		}
		flat := make(map[string]any, len(wt)+2)
		for k, v := range wt {
			flat[k] = v
		}
		flat["id"] = wsID
		flat["chatId"] = c["id"]
		out[wsID] = flat
	}
	return out
}

// PushProviderState injects a scripted provider poll result for wsID directly
// into the workspace aggregate, bypassing the real GitHub/GitLab provider
// engine. The resulting status transition (pr-open → pr-merged, protected →
// locked, etc.) is persisted projection-synchronously (Asynx SendWait) and
// broadcast as a WorkspaceDTO on the Workspaces WS stream. This is the
// mock-provider seam (spec §11) — analogous to PushGit/PushLSP — that lets a
// test drive PR-status transitions deterministically without a real provider.
func (e *Env) PushProviderState(
	t *testing.T,
	wsID string,
	state ProviderState,
) {
	t.Helper()
	_, err := e.app.Repositories.Workspace.SyncProviderState(
		context.Background(),
		wsrepo.ProviderInput{
			ID:             wsID,
			Protected:      state.Protected,
			HasPR:          state.HasPR,
			PRStatus:       state.PRStatus,
			PRUrl:          state.PRUrl,
			PRTitle:        state.PRTitle,
			PRTargetBranch: state.PRTargetBranch,
		},
		Now(),
	)
	require.NoError(t, err, "PushProviderState: SyncProviderState")
}

// DecodeJSON decodes the response body into dest and closes the body.
func DecodeJSON(
	t *testing.T,
	resp *http.Response,
	dest any,
) {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(
		t,
		err,
	)
	require.NoError(
		t,
		json.Unmarshal(
			raw,
			dest,
		),
		"body: %s",
		string(raw),
	)
}

// RequireStatus fails the test if the response status doesn't match.
func RequireStatus(
	t *testing.T,
	resp *http.Response,
	want int,
) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf(
			"expected status %d got %d: %s",
			want,
			resp.StatusCode,
			string(body),
		)
	}
}

func wsURL(
	base string,
	path string,
) string {
	return "ws" + strings.TrimPrefix(base, "http") + path
}

// Now returns a stable deterministic time for test commands.
func Now() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}

// NowPlus returns Now() advanced by d.
func NowPlus(
	d time.Duration,
) time.Time {
	return Now().Add(d)
}

// Ctx returns a background context.
func Ctx() context.Context {
	return context.Background()
}

// MustJSON marshals v to JSON or panics if marshalling fails.
func MustJSON(
	v any,
) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("MustJSON: %v", err))
	}
	return string(b)
}
