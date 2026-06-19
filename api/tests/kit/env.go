//go:build integration

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
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

// Env is the full integration test environment: a real HTTP+WS server backed by
// real SQLite stores, plus the app and engine containers for direct usecase calls.
//
// All WS helpers use deadline-based reads (no time.Sleep).
type Env struct {
	// URL is the HTTP base URL of the test server (e.g. "http://127.0.0.1:PORT").
	URL string
	// app exposes the application layer for direct usecase and repository calls.
	app *app.Container
	// engine exposes the engine layer for direct engine calls.
	engine *engine.Container
	// v0c exposes test helpers for the v0 API container (WaitRegistered / PushLSP).
	v0c      *v0.Container
	homeDir  string
	adapters *adapter.Container
}

// BuildEnv spins up the full server stack (engine → adapter → app → api),
// wires an httptest.Server on an ephemeral port, and registers cleanup.
// Each call returns a fully isolated environment backed by its own temp dir.
func BuildEnv(
	t *testing.T,
) *Env {
	t.Helper()
	return BuildEnvAt(t, t.TempDir())
}

// BuildEnvAt spins up the full server stack (engine → adapter → app → api)
// using a caller-supplied homeDir instead of a fresh temp directory. This
// allows two successive calls to share the same SQLite database, which is
// required for crash-recovery tests where a second "server" must read rows
// written by the first.
func BuildEnvAt(
	t *testing.T,
	homeDir string,
) *Env {
	t.Helper()
	ctx := context.Background()

	eng, err := engine.New(
		ctx,
		engine.WithHomeDir(homeDir),
	)
	require.NoError(
		t,
		err,
	)

	adapters, err := adapter.New(adapter.WithHomeDir(homeDir))
	require.NoError(
		t,
		err,
	)
	t.Cleanup(func() { _ = adapters.Close() })

	appContainer, err := app.New(
		ctx,
		eng,
		adapters,
	)
	require.NoError(
		t,
		err,
	)

	router := gin.New()
	apiContainer := v0.New(
		appContainer,
		eng,
	)
	apiContainer.Register(router.Group("/v0"))

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &Env{
		URL:      srv.URL,
		app:      appContainer,
		engine:   eng,
		v0c:      apiContainer,
		homeDir:  homeDir,
		adapters: adapters,
	}
}

// Close eagerly flushes and releases the adapter's SQLite connections.
// This is required when two Envs share the same homeDir (crash-recovery tests):
// env1 must release its file handles before env2 opens the same database files,
// even though WAL mode permits concurrent readers.
// The deferred t.Cleanup registered by BuildEnvAt will call Close a second time,
// which is safe — subsequent calls are no-ops because the underlying sql.DB is
// already closed.
func (e *Env) Close(
	t *testing.T,
) {
	t.Helper()
	if err := e.adapters.Close(); err != nil {
		t.Logf("kit.Env.Close: %v", err)
	}
}

// HomeDir returns the home directory path used by this Env.
func (e *Env) HomeDir() string { return e.homeDir }

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
	w := Dial(t, wsURL(e.URL, "/v0/projects"))
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
	w := Dial(t, wsURL(e.URL, "/v0/projects/"+projectID+"/repos"))
	e.v0c.WaitNReposRegistered(1)
	return w
}

// DialWorkspaces opens a WS watcher on the repo-scoped Workspaces stream
// (WS /v0/projects/:projectId/repos/:repoId/workspaces) and blocks until
// registration. A repo-scope subscriber receives all of the repo's workspaces
// via hierarchical prefix matching (spec §5).
func (e *Env) DialWorkspaces(
	t *testing.T,
	projectID string,
	repoID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, wsURL(e.URL, "/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces"))
	e.v0c.WaitNWorkspacesRegistered(1)
	return w
}

// DialWorkspace opens a WS watcher on the exact-workspace Workspaces stream
// (WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId) and blocks until
// registration. An exact-scope subscriber receives only that workspace's DTOs.
func (e *Env) DialWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, wsURL(e.URL, "/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces/"+wsID))
	e.v0c.WaitNWorkspacesRegistered(1)
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
	w := Dial(t, wsURL(e.URL, e.wsScope(projectID, repoID, wsID)+"/threads"))
	e.v0c.WaitNThreadsRegistered(1)
	return w
}

// DialTerminals opens a WS watcher on the workspace-scoped Terminals lifecycle
// stream and blocks until registration. The subscriber receives that
// workspace's TerminalSessionDTOs (snapshot + live). This is the lifecycle
// topic, NOT the raw PTY stream (.../terminals/:sessionId/ws).
func (e *Env) DialTerminals(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, wsURL(e.URL, e.wsScope(projectID, repoID, wsID)+"/terminals"))
	e.v0c.WaitNTerminalsRegistered(1)
	return w
}

// DialGit opens a WS watcher on the workspace-scoped, co-located Git status
// stream (.../workspaces/:wsId/git/status) and blocks until registration. The
// git broadcaster uses a flat wsId namespace, so scoping is implicit in the
// path (no query params).
func (e *Env) DialGit(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, wsURL(e.URL, e.wsScope(projectID, repoID, wsID)+"/git/status"))
	e.v0c.WaitNGitRegistered(1)
	return w
}

// DialFiles opens a WS watcher on the workspace-scoped, co-located Files stream
// (.../workspaces/:wsId/files/ws, change-only, no snapshot) and blocks until
// registration.
func (e *Env) DialFiles(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, wsURL(e.URL, e.wsScope(projectID, repoID, wsID)+"/files/ws"))
	e.v0c.WaitNFilesRegistered(1)
	return w
}

// DialLSP opens a WS watcher on the workspace-scoped, co-located LSP stream
// (.../workspaces/:wsId/lsp/ws) and blocks until registration.
func (e *Env) DialLSP(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, wsURL(e.URL, e.wsScope(projectID, repoID, wsID)+"/lsp/ws"))
	e.v0c.WaitNLSPRegistered(1)
	return w
}

// wsScope builds the hierarchical workspace path prefix shared by all
// workspace-scoped routes.
func (e *Env) wsScope(
	projectID string,
	repoID string,
	wsID string,
) string {
	return "/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces/" + wsID
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
	return w.ReadUntil(
		t,
		timeout,
		func(msg map[string]any) bool {
			if msg["id"] != wsID {
				return false
			}
			return pred(msg)
		},
	)
}

// WaitForWorkspaceState reads WS events from w until a WorkspaceDTO with the
// given id reaches the given status. Returns the decoded DTO map. A blank status
// matches the omitempty "" state (commits, no PR).
func WaitForWorkspaceState(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	status string,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return w.ReadUntil(
		t,
		timeout,
		func(msg map[string]any) bool {
			if msg["id"] != wsID {
				return false
			}
			got, _ := msg["status"].(string)
			return got == status
		},
	)
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
	msg := w.ReadUntil(
		t,
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
	resp, err := http.Get(e.URL + path)
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
	resp, err := http.Post(
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
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)
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

// CreateWorkspace creates a workspace via
// POST /v0/projects/:projectId/repos/:repoId/workspaces and returns the
// server-assigned UUID. Creation is async (202 + empty body, spec §4): the id is
// learned from the WorkspaceDTO{status:"new"} delivered on the repo-scoped
// Workspaces WS stream. The WS is dialled BEFORE the POST so the create
// broadcast is never missed.
func (e *Env) CreateWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
) string {
	t.Helper()
	return e.createWorkspace(t, projectID, repoID, branch, "")
}

// CreateChildWorkspace creates a child workspace under parentID and returns the
// UUID. See CreateWorkspace for the 202 + WS-learning semantics.
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
	w := e.DialWorkspaces(t, projectID, repoID)
	body := map[string]any{"branch": branch}
	if parentID != "" {
		body["parentId"] = parentID
	}
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces", body)
	RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	msg := w.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		return m["branch"] == branch && m["status"] == string(domain.WorkspaceStatusNew)
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "createWorkspace: WorkspaceDTO must carry an id")
	return id
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
