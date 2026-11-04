//go:build integration

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Env is the full integration environment: the real adapter+app+api stack over a Unix socket
// (the production transport) on real SQLite under an isolated home, plus the app and engine
// containers. All WS helpers use deadline-based reads.
type Env struct {
	// URL is the fixed pseudo base URL of the test server ("http://crowbar"). The
	// host is a placeholder: every request is routed to the Unix socket by the
	// env's client/dialer, never by DNS/TCP.
	URL string
	// app exposes the application layer for direct usecase and repository calls.
	app *app.Container
	// engine exposes the engine layer for direct engine calls.
	engine   *engine.Container
	homeDir  string
	adapters *adapter.Container
	session  *migrate.Session

	// server + listener serve the router over socketPath; client + dialer reach it. closeOnce
	// makes exactly one teardown body run, whichever Close variant or cleanup fires first.
	server     *http.Server
	listener   net.Listener
	socketPath string
	client     *http.Client
	dialer     *websocket.Dialer
	closeOnce  sync.Once
	// quiesceMu orders the kit's own writes (read side) against Quiesce
	// (write side); see Quiesce.
	quiesceMu sync.RWMutex
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

// tempHome makes an isolated home with a TOLERANT teardown, not t.TempDir(): detached async work
// can write a storages file just after the adapter closes, racing RemoveAll. That race is benign,
// so removal is best-effort and a cleanup error never fails the test.
func tempHome(
	t *testing.T,
) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kitenv-")
	require.NoError(t, err, "kit: create temp home")
	t.Cleanup(func() {
		var err error
		for i := 0; i < 50; i++ {
			if err = os.RemoveAll(dir); err == nil {
				return
			}
			runtime.Gosched()
		}
		// Best-effort: a residual handle from a detached write may still hold a
		// file; the OS reaps the temp tree. Do not fail the test on cleanup.
		t.Logf("kit: remove temp home: %v", err)
	})
	return dir
}

// BuildEnvAt spins up the full stack on a caller-supplied homeDir, so a second call can read
// the first's SQLite rows (crash-recovery tests). Its graceful-Close cleanup is idempotent.
func BuildEnvAt(
	t *testing.T,
	homeDir string,
) *Env {
	t.Helper()
	env, err := NewEnvWithHome(homeDir)
	require.NoError(t, err, "kit: build env")
	// Registered after tempHome's RemoveAll so it runs BEFORE it (LIFO); closeOnce makes it a
	// no-op after an explicit Close variant.
	t.Cleanup(func() { env.Close(t) })
	return env
}

// sockCounter uniquifies per-Env socket paths within a process so a restart
// (a second Env over the same home) binds its OWN short socket rather than
// contending on the first env's path.
var sockCounter atomic.Uint64

// kitSocketPath returns a short unique socket path in the OS temp dir: macOS caps sun_path
// at 104 bytes, so it must not live under the long temp home.
func kitSocketPath() string {
	return filepath.Join(
		os.TempDir(),
		fmt.Sprintf("cbk-%d-%d.sock", os.Getpid(), sockCounter.Add(1)),
	)
}

// NewEnvWithHome is the restart primitive: it serves the real stack rooted at homeDir and
// registers no cleanup (the caller closes). A second call on the same homeDir is a restart.
func NewEnvWithHome(
	homeDir string,
) (*Env, error) {
	ctx := context.Background()
	session, err := BootHome(ctx, homeDir)
	if err != nil {
		return nil, releaseHome(session, err)
	}

	eng, err := engine.New(ctx, engine.WithHomeDir(homeDir))
	if err != nil {
		return nil, releaseHome(session, fmt.Errorf("kit: engine: %w", err))
	}

	adapters, err := adapter.New(adapter.WithHomeDir(homeDir))
	if err != nil {
		eng.Close()
		return nil, releaseHome(session, fmt.Errorf("kit: adapter: %w", err))
	}

	appContainer, err := app.New(ctx, eng, adapters, session, v0.EncodeEntity, v0.NewSidebar)
	if err != nil {
		eng.Close()
		return nil, releaseHome(session, errors.Join(fmt.Errorf("kit: app: %w", err), adapters.Close()))
	}

	router := gin.New()
	apiContainer := v0.New(appContainer, eng, "")
	apiContainer.Register(router.Group("/v0"))

	socketPath := kitSocketPath()
	listener, err := transports.NewSocket("unix://" + socketPath)
	if err != nil {
		appContainer.Close()
		eng.Close()
		return nil, releaseHome(session, errors.Join(fmt.Errorf("kit: gateway: %w", err), adapters.Close()))
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
		homeDir:    homeDir,
		adapters:   adapters,
		session:    session,
		server:     server,
		listener:   listener,
		socketPath: socketPath,
		client:     client,
		dialer:     dialer,
	}, nil
}

// Close shuts down GRACEFULLY in the daemon's drain order: HTTP, reactors and asynx singletons,
// realtime and engine children, then checkpoint and close the DBs so a second Env can reopen
// the home. Runs at most once (closeOnce).
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
		e.releaseStores(t, "Close")
		e.closeListener(t, "Close")
	})
}

// CloseCrashing simulates SIGKILL: no server or app Shutdown, so reactors and asynx work are
// abandoned mid-flight, but the DBs and OS handles close so a second Env can drive recovery.
// Shares closeOnce with the other variants (call exactly one).
func (e *Env) CloseCrashing(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		e.closeAbruptly(t) // drop the listener + active conns, no drain.
		e.app.Close()      // release realtime FDs (watchers/LSP) — not a data drain.
		e.engine.Close()
		e.releaseStores(t, "CloseCrashing")
		e.closeListener(t, "CloseCrashing")
	})
}

// CloseWithoutKilling tears down gracefully but LEAVES the engine's PTY shells and LSP servers
// running, for tests on processes surviving a daemon teardown. Shares closeOnce (call one).
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
		e.releaseStores(t, "CloseWithoutKilling")
		e.closeListener(t, "CloseWithoutKilling")
	})
}

// SocketPath returns the Unix domain socket path this Env serves on.
func (e *Env) SocketPath() string { return e.socketPath }

// HomeDir returns the home directory path used by this Env.
func (e *Env) HomeDir() string { return e.homeDir }

// ShutdownTerminal persists every live session as "suspended"; call it before Close when
// simulating a restart so the next BuildEnvAt can restore them.
func (e *Env) ShutdownTerminal() {
	if e.engine != nil && e.engine.Terminal != nil {
		e.engine.Terminal.Shutdown()
	}
}

// WorktreePath returns the server's own persisted worktree path from the read model rather than
// re-deriving the slug (worktreepath is unreachable from the kit). projectID/repoID are unused;
// the path is keyed by wsID.
func (e *Env) WorktreePath(
	_ string,
	_ string,
	wsID string,
) string {
	ws, err := e.app.Repositories.Workspace.Get(context.Background(), wsID)
	if err != nil {
		panic(fmt.Sprintf("kit: WorktreePath: get workspace %q: %v", wsID, err))
	}
	return ws.WorktreePath
}

// ProviderState is a scripted provider poll result for PushProviderState, mirroring the
// aggregate's inputs without depending on the repository's ProviderInput.
type ProviderState struct {
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// DialTerminalPTY watches the raw PTY stream, a direct engine pipe rather than a broadcaster
// topic, so there is no WaitNRegistered gate.
func (e *Env) DialTerminalPTY(
	t *testing.T,
	chatID string,
	sessionID string,
) *WSWatcher {
	t.Helper()
	return Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/terminals/"+sessionID+"/ws"))
}

// chatScope builds the FLAT chat path prefix shared by all chat-scoped routes.
// Chat ids are globally unique, so nothing below it re-nests under a
// project/repo pair.
func (e *Env) chatScope(
	chatID string,
) string {
	return "/v0/chats/" + chatID
}

// OwningChatID returns the chat that OWNS the workspace (minted chat-first, so it already exists).
// It reads in-process because workspaces are no longer addressable over HTTP, using the same
// ListChatsByWorkspace + domain.ResolveOwningChat calls as the wire resolver.
func (e *Env) OwningChatID(
	t *testing.T,
	wsID string,
) string {
	t.Helper()
	ctx := context.Background()
	// The chat-first mint is an asynx write; drain the projections so the read
	// below sees the row rather than racing it (no polling, no sleep).
	e.Quiesce()

	rows, err := e.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, wsID)
	require.NoError(t, err, "OwningChatID: list the chats holding %s", wsID)
	owner, ok := domain.ResolveOwningChat(rows)
	require.Truef(
		t,
		ok,
		"workspace %s must be held by an owning chat for the chat-scoped routes",
		wsID,
	)
	return owner.ID
}

// WorktreeFrame flattens a REST chat row's `worktree` object, restoring `id`, `chatId` and
// `repoId` from the row; a row with no worktree is returned unchanged.
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

// WaitForWorkspace reads the projects scope until the workspace entity wsID
// satisfies pred, returning its row.
func WaitForWorkspace(
	t *testing.T,
	w *EntityWatcher,
	wsID string,
	timeout time.Duration,
	pred func(map[string]any) bool,
) map[string]any {
	t.Helper()
	return w.Next(t, timeout, stream.KindWorkspace, func(row map[string]any) bool {
		return row["id"] == wsID && pred(row)
	})
}

// WaitForWorkspaceState reads the projects scope until workspace wsID reaches
// status. A blank status matches the omitempty "" state; "deleted" matches the
// gone frame a tombstoned workspace is sent as.
func WaitForWorkspaceState(
	t *testing.T,
	w *EntityWatcher,
	wsID string,
	status string,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return WaitForWorkspace(t, w, wsID, timeout, func(row map[string]any) bool {
		if gone, _ := row["gone"].(bool); gone {
			return status == string(domain.WorkspaceStatusDeleted)
		}
		got, _ := row["status"].(string)
		return got == status
	})
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

// RegisterProject creates a project via POST /v0/projects and returns its id from the projects
// scope, held BEFORE the POST so the created row is never missed.
func (e *Env) RegisterProject(
	t *testing.T,
	name string,
	path string,
) string {
	t.Helper()
	w := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	msg := w.Next(t, 10*time.Second, stream.KindProject, func(m map[string]any) bool {
		p, _ := m["path"].(string)
		return p == path
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "RegisterProject: ProjectDTO must carry an id")
	return id
}

// RegisterRepo creates a repo and returns its id from the projects scope, held BEFORE the POST
// so the row is never missed. path may be empty for repos that need no git operations.
func (e *Env) RegisterRepo(
	t *testing.T,
	projectID string,
	name string,
	path string,
) string {
	t.Helper()
	w := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	msg := w.Next(t, 10*time.Second, stream.KindRepo, func(m map[string]any) bool {
		return m["name"] == name && m["projectId"] == projectID
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "RegisterRepo: RepoDTO must carry an id")
	return id
}

// CreateWorkspace creates a worktree on a caller-named branch through the USECASE: naming the
// branch is a fixture need no HTTP route offers. It makes the live import path's CreateChild
// call, minting and attaching the owning chat itself (see createWorkspace).
func (e *Env) CreateWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
) string {
	t.Helper()
	wsID, _ := e.createWorkspace(t, projectID, repoID, branch, "")
	return wsID
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
	wsID, _ := e.createWorkspace(t, projectID, repoID, branch, parentID)
	return wsID
}

// createWorkspace mints and attaches the owning chat via the import paths' three-verb
// primitive: nothing would give an orphaned CreateChild call one afterwards.
func (e *Env) createWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (wsID, chatID string) {
	t.Helper()
	wsID, chatID = e.createWorkspaceWrites(t, projectID, repoID, branch, parentID)
	// The create's own writes are asynx commands; drain them so a caller that
	// reads the row (or dials its chat) next sees it rather than racing it.
	e.Quiesce()
	return wsID, chatID
}

// createWorkspaceWrites issues a create's commands inside the kit's write
// window: a Quiesce from a sibling goroutine waits for them instead of making
// the dispatcher refuse them (see Env.quiesceMu).
func (e *Env) createWorkspaceWrites(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (wsID, chatID string) {
	t.Helper()
	e.quiesceMu.RLock()
	defer e.quiesceMu.RUnlock()
	ctx := context.Background()
	repo, err := e.app.GORM.Repositories.FindByKey(ctx, repoID)
	require.NoError(t, err, "createWorkspace: read repo %s", repoID)
	require.NotNil(t, repo, "createWorkspace: repo %s must exist", repoID)
	// ParentBranch must be supplied: resolveInherited only defaults it when neither RepoID nor
	// RepoPath is named, and a blank one makes `git worktree add` die.
	parentBranch := repo.DefaultBranch
	if parentID != "" {
		parent, perr := e.app.Repositories.Workspace.Get(ctx, parentID)
		require.NoError(t, perr, "createWorkspace: read parent workspace %s", parentID)
		parentBranch = parent.Branch
	}

	mintedChatID, err := e.app.Usecases.AgentChatFolder.MintOwningChat(ctx, "", parentID)
	require.NoError(t, err, "createWorkspace: mint the owning chat")

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
	if err != nil {
		require.NoError(t, e.app.Usecases.AgentChatFolder.DiscardOwningChat(ctx, mintedChatID),
			"createWorkspace: discard the owning chat of a workspace that was never created")
	}
	require.NoErrorf(t, err, "createWorkspace: create %q under %q", branch, parentID)
	require.NotEmpty(t, ws.ID, "createWorkspace: the created workspace must carry an id")

	require.NoError(t, e.app.Usecases.AgentChatFolder.AttachOwningWorkspace(ctx, mintedChatID, ws),
		"createWorkspace: attach the minted chat to its workspace")
	return ws.ID, mintedChatID
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
	return e.createWorkspace(t, projectID, repoID, branch, parentID)
}

// ImportedRepo bundles a full project+repo import's ids: project, repo, the adopted main
// workspace, and ChatID, its owning chat, which the import already minted.
type ImportedRepo struct {
	ProjectID   string
	RepoID      string
	WorkspaceID string
	ChatID      string
	RepoPath    string
}

// Quiesce blocks until every asynx projection drained: the read-your-writes barrier, since the
// store read model can trail the WS frame. It holds quiesceMu exclusively because the barrier
// refuses commands dispatched while it waits; concurrent kit writes finish first or wait.
func (e *Env) Quiesce() {
	e.quiesceMu.Lock()
	defer e.quiesceMu.Unlock()
	e.app.Repositories.WaitQuiescent()
}

// QuiesceReactors is Quiesce plus the join of every post-commit reactor, which detaches into its
// own goroutine (so Quiesce sees it done at spawn); it is the barrier for cascades like DELETE.
func (e *Env) QuiesceReactors() {
	e.app.Repositories.QuiesceReactors(context.Background())
}

// HoldReactors parks every reactor admitted from now on at the drain gate, so a test can crash
// with a purge deterministically pending. Nothing releases it; crash next.
func (e *Env) HoldReactors() {
	e.app.Repositories.Drain().Gate.Hold()
}

// ImportRepo creates (or inits) a git repo, imports it as a project and runs the full repo import,
// returning the ids and path; the adopted workspace comes from the WS broadcast, dialled first.
func (e *Env) ImportRepo(
	t *testing.T,
	name string,
	path string,
) ImportedRepo {
	t.Helper()
	if path == "" {
		path = InitRepo(t)
	}
	// Cleanups run last-registered first: without this the repo's TempDir would
	// be removed while the daemon still works in it.
	t.Cleanup(func() { e.Close(t) })
	projectID := e.RegisterProject(t, name, path)
	// The importer names the repo from its directory, so match the RepoDTO by projectId; hold
	// projects BEFORE the POST so the row is never missed.
	reposWS := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": name,
		"path": path,
	})
	// The repo row lands as the import starts; its worktrees by the time the
	// import's operation settles.
	e.AwaitOperation(t, resp, 60*time.Second)
	repo := reposWS.Next(t, 10*time.Second, stream.KindRepo, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "ImportRepo: import must broadcast a RepoDTO with an id")

	// The adopted main worktree is read back once the import settled: Quiesce
	// drains the projections, then the chat list is read.
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

// ListChats reads a repo's chat rows over REST: each worktree-owning row carries its git
// state inline.
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

// DeleteWorkspaceCascade tombstones a workspace and starts its purge cascade, for a test that
// needs a tombstone as a PRECONDITION: DELETE .../chats/:id also purges the owning chat.
func (e *Env) DeleteWorkspaceCascade(
	t *testing.T,
	wsID string,
) {
	t.Helper()
	require.NoError(
		t,
		e.app.Usecases.Workspace.DeleteCascade(context.Background(), wsID, domain.KeepWorkAtRisk),
		"DeleteWorkspaceCascade: %s",
		wsID,
	)
}

// WorkspaceRow returns a workspace's status in the DURABLE read model and whether its row exists.
// In-process because no wire read reaches a row whose owning chat was purged; it reads the same
// store the boot orphan-sweep lists from.
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

// RecordRepoDeleteIntent writes the durable intent a repo delete records
// before any teardown — the state a crash right after that write leaves.
func (e *Env) RecordRepoDeleteIntent(
	t *testing.T,
	repoID string,
) {
	t.Helper()
	ctx := context.Background()
	repo, err := e.app.GORM.Repositories.FindByKey(ctx, repoID)
	require.NoError(t, err)
	require.NotNil(t, repo, "RecordRepoDeleteIntent: repo %s", repoID)
	repo.Deleting = true
	require.NoError(t, e.app.GORM.Repositories.Save(ctx, *repo))
}

// RepoRow reports whether repoID's row is still in the repository store.
func (e *Env) RepoRow(
	t *testing.T,
	repoID string,
) bool {
	t.Helper()
	repo, err := e.app.GORM.Repositories.FindByKey(context.Background(), repoID)
	require.NoError(t, err)
	return repo != nil
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

// PushProviderState injects a scripted provider poll result into the workspace aggregate,
// bypassing the provider engine; the transition is persisted (SendWait) and broadcast, so tests
// drive PR-status deterministically.
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
