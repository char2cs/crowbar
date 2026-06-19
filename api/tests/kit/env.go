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

// DialWorkspaces opens a WS watcher on /v0/ws/workspaces and blocks until the
// server has registered this specific client (prevents broadcast races).
//
// Each call drains exactly one registration token from the per-registration
// semaphore, so multiple calls on the same Env are safe — each blocks until the
// client just dialled appears in the broadcaster's map.
func (e *Env) DialWorkspaces(
	t *testing.T,
	queryParams string,
) *WSWatcher {
	t.Helper()
	url := wsURL(
		e.URL,
		"/v0/ws/workspaces"+queryParams,
	)
	w := Dial(
		t,
		url,
	)
	e.v0c.WaitNWorkspacesRegistered(1)
	return w
}

// DialGit opens a WS watcher on /v0/ws/git and blocks until the server has
// registered this specific client (prevents broadcast races).
//
// Each call drains exactly one registration token from the per-registration
// semaphore, so multiple calls on the same Env are safe — each blocks until the
// client just dialled appears in the broadcaster's map.
func (e *Env) DialGit(
	t *testing.T,
	queryParams string,
) *WSWatcher {
	t.Helper()
	url := wsURL(
		e.URL,
		"/v0/ws/git"+queryParams,
	)
	w := Dial(
		t,
		url,
	)
	e.v0c.WaitNGitRegistered(1)
	return w
}

// DialFiles opens a WS watcher on /v0/ws/files (change-only, no snapshot) and
// blocks until the server has registered this specific client (prevents broadcast races).
//
// Each call drains exactly one registration token from the per-registration
// semaphore, so multiple calls on the same Env are safe — each blocks until the
// client just dialled appears in the broadcaster's map.
func (e *Env) DialFiles(
	t *testing.T,
	queryParams string,
) *WSWatcher {
	t.Helper()
	url := wsURL(
		e.URL,
		"/v0/ws/files"+queryParams,
	)
	w := Dial(
		t,
		url,
	)
	e.v0c.WaitNFilesRegistered(1)
	return w
}

// DialLSP opens a WS watcher on /v0/ws/lsp and blocks until the server has
// registered this specific client (prevents broadcast races).
//
// Each call drains exactly one registration token from the per-registration
// semaphore, so multiple calls on the same Env are safe — each blocks until the
// client just dialled appears in the broadcaster's map.
func (e *Env) DialLSP(
	t *testing.T,
	queryParams string,
) *WSWatcher {
	t.Helper()
	url := wsURL(
		e.URL,
		"/v0/ws/lsp"+queryParams,
	)
	w := Dial(
		t,
		url,
	)
	e.v0c.WaitNLSPRegistered(1)
	return w
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

// RegisterRepo creates a repo record via POST /v0/repos.
// path may be empty for repos that don't need git operations.
func (e *Env) RegisterRepo(
	t *testing.T,
	id string,
	path string,
) {
	t.Helper()
	resp := e.POST(t, "/v0/repos", map[string]any{
		"id":        id,
		"projectId": "p1",
		"name":      id,
		"path":      path,
	})
	RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

// CreateWorkspace creates a workspace via POST /v0/workspaces and returns the
// server-assigned UUID. repoID must already be registered via RegisterRepo.
func (e *Env) CreateWorkspace(
	t *testing.T,
	repoID string,
	branch string,
) string {
	t.Helper()
	resp := e.POST(t, "/v0/workspaces", map[string]any{
		"repoId": repoID,
		"branch": branch,
	})
	RequireStatus(t, resp, http.StatusCreated)
	return MutationID(t, resp)
}

// CreateChildWorkspace creates a child workspace under parentID and returns the UUID.
func (e *Env) CreateChildWorkspace(
	t *testing.T,
	repoID string,
	branch string,
	parentID string,
) string {
	t.Helper()
	resp := e.POST(t, "/v0/workspaces", map[string]any{
		"repoId":   repoID,
		"branch":   branch,
		"parentId": parentID,
	})
	RequireStatus(t, resp, http.StatusCreated)
	return MutationID(t, resp)
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
