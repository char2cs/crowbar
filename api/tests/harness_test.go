//go:build integration

// Package tests holds the end-to-end integration suite for the Crowbar /v0 API.
// Each test boots the REAL wired backend (engine -> adapter -> app -> api) over
// an httptest.Server and drives the DoD walkthrough at the HTTP and WebSocket
// level. The frontend is stubbed, so this suite is the authoritative end-to-end
// verification of the backend.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// harness owns the booted server and its base URL. It exposes typed helpers for
// the REST envelope and the WebSocket topics so individual suites stay terse.
type harness struct {
	t         *testing.T
	server    *httptest.Server
	url       string
	home      string
	app       *app.Container
	stopOnce  sync.Once
	stop      func()
	crashDie  func()
	drainOnce sync.Once
	drainDown func()
}

// newHarness boots the full backend over an httptest.Server rooted at a
// per-test temp home dir. A stub fs.FS stands in for the unbuilt web/dist embed.
func newHarness(
	t *testing.T,
) *harness {
	t.Helper()
	return newHarnessAt(t, t.TempDir())
}

// newHarnessAt boots the full backend rooted at an EXPLICIT crowbar home. It
// backs the daemon-restart suites: shut a harness down (harness.shutdown) and
// boot a second one over the SAME home to exercise every startup path that
// reads persisted state (terminal restore, agent boot reconcile) exactly as a
// real daemon restart does.
func newHarnessAt(
	t *testing.T,
	home string,
) *harness {
	t.Helper()
	ctx := context.Background()

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	appContainer, err := app.New(ctx, engines, adapters)
	require.NoError(t, err)

	apiContainer, err := crowbarapi.New(appContainer, engines, fstest.MapFS{})
	require.NoError(t, err)

	srv := httptest.NewServer(apiContainer.Handler())

	h := &harness{
		t:    t,
		home: home,
		app:  appContainer,
	}
	h.server = srv
	h.url = srv.URL
	// drainDown is the daemon's ORDERED GRACEFUL DRAIN — internal.Container.Run's
	// ctx.Done branch, verbatim: stop serving, then quiesce every asynchronous
	// writer and drain every aggregate, all while the DBs are still OPEN.
	//
	// Splitting it out of stop is what lets a test look at the state the daemon
	// commits ON ITS WAY DOWN (harness.drain), in the one window where that state is
	// still readable — the DBs it was written to have not been closed yet. Shutdown
	// is passed a cancel-free context on purpose: the production 5s deadline is a
	// BOUND on a wedge, never a synchronisation device, and a test that leaned on it
	// would be timing-dependent by construction.
	h.drainDown = func() {
		srv.Close()
		_ = appContainer.Shutdown(context.Background())
	}
	// Shutdown order mirrors the daemon's (internal.Container.Run then .Close): the
	// graceful drain above first, then the app (realtime), then the engines (terminal
	// Shutdown suspends+persists live PTYs — the restart-restore contract), then the
	// adapters (event/store DBs) LAST, once every writer is quiesced.
	h.stop = func() {
		h.drain()
		appContainer.Close()
		engines.Close()
		_ = adapters.Close()
	}
	// crashDie is the POWER-CUT order, and the inversion is the whole point: the
	// durable store goes FIRST, so nothing that happens afterwards can be recorded.
	// See harness.crash.
	h.crashDie = func() {
		srv.Close()
		_ = adapters.Close()
		appContainer.Close()
		engines.Close()
	}
	t.Cleanup(h.shutdown)

	return h
}

// drain runs the daemon's graceful-shutdown DRAIN and stops there, exactly once:
// serving stops and every asynchronous writer is quiesced, but the DBs stay open
// and the harness's app container stays readable. It is the seam for asserting
// what a graceful stop RECORDS on the way down (see drainDown).
//
// A subsequent shutdown() completes the teardown; a crash() bypasses it entirely
// (a power cut drains nothing).
func (h *harness) drain() {
	h.drainOnce.Do(h.drainDown)
}

// shutdown tears the booted backend down exactly once — as a graceful daemon
// stop would. Safe to call from a test AND from the registered cleanup.
func (h *harness) shutdown() {
	h.stopOnce.Do(h.stop)
}

// crash tears the booted backend down as a daemon DEATH would — a SIGKILL, a
// SIGQUIT from the watchdog, a panic — rather than a graceful stop. It is the
// precondition for every boot-reconciliation test, and a graceful shutdown cannot
// stand in for it.
//
// The difference is which facts survive. A graceful stop lets the terminal engine
// kill each PTY and lets the resulting exit callbacks RECORD those deaths (agent
// runner rows are Exited on the way down). A crash records NOTHING: the process is
// simply gone, and what the next daemon comes back to is a durable sqlite table
// full of LIVE runner rows describing vendor CLIs that no longer exist. That stale
// row is the entire subject of ReconcileRunnersOnBoot — and it is the thing that
// bricks a chat (LiveRunnerForChat hands ResumeChat a corpse, which it returns as a
// silent no-op), so a test that never produces one tests nothing.
//
// It is modelled by closing the DB adapters FIRST. Everything downstream then
// behaves exactly as it does in a real crash: the engines still tear down (so no
// `cat` child process leaks out of the test), their exit callbacks still fire, and
// every write those callbacks attempt fails against a store that is already gone —
// which is precisely what "nothing recorded the death" means. The warnings they log
// on the way out are the expected sound of a crash, not a failure.
//
// Safe to call from a test; the registered graceful cleanup becomes a no-op.
func (h *harness) crash() {
	h.stopOnce.Do(h.crashDie)
}

// Quiesce blocks until every per-type asynx projection has drained (dispatch
// queue idle + all projection handlers run) — the deterministic
// read-your-writes barrier (app/repositories.Container.WaitQuiescent's doc
// comment). Some mutations are observable immediately on their own HTTP
// response (e.g. agent Create returns the aggregate's id straight from the
// write path), but a projection's store/list read model (e.g. agent List/Get)
// is an INDEPENDENT async projection that can trail it; call Quiesce after a
// mutation so a subsequent plain-REST read of that projection is guaranteed
// consistent, with no polling and no timeouts (mirrors kit.Env.Quiesce for
// the api/tests/integration/... suites).
func (h *harness) Quiesce() {
	h.app.Repositories.WaitQuiescent()
}

// QuiesceReactors is Quiesce PLUS the join of every post-commit REACTOR the
// mutation set in motion, then a final fold of what those reactors dispatched.
// It mirrors kit.Env.QuiesceReactors (see its doc comment for why all three steps
// are load-bearing) and is the barrier for the async cascades — above all DELETE —
// whose effect lands outside the aggregate and is therefore invisible to a plain
// projection drain: a reactor detaches into its own goroutine, so WaitQuiescent
// sees its handler "complete" the moment the goroutine is spawned, long before
// the purge it spawned has finished.
func (h *harness) QuiesceReactors() {
	h.app.Repositories.WaitQuiescent()
	h.app.Repositories.Drain().Gate.WaitIdle(context.Background())
	h.app.Repositories.WaitQuiescent()
}

// get issues GET path, asserts the success envelope, and decodes data into out.
func (h *harness) get(
	path string,
	out any,
) {
	h.do(http.MethodGet, path, nil, http.StatusOK, out)
}

// post issues POST path with a JSON body and decodes the data envelope into out
// asserting the given status.
func (h *harness) post(
	path string,
	body any,
	wantStatus int,
	out any,
) {
	h.do(http.MethodPost, path, body, wantStatus, out)
}

// put issues PUT path with a JSON body and decodes the data envelope into out.
func (h *harness) put(
	path string,
	body any,
	out any,
) {
	h.do(http.MethodPut, path, body, http.StatusOK, out)
}

// patch issues PATCH path with a JSON body and decodes the data envelope.
func (h *harness) patch(
	path string,
	body any,
	out any,
) {
	h.do(http.MethodPatch, path, body, http.StatusOK, out)
}

// del issues DELETE path with an optional JSON body and asserts the status. It
// decodes the data envelope into out when wantStatus is not 204.
func (h *harness) del(
	path string,
	body any,
	wantStatus int,
	out any,
) {
	if wantStatus == http.StatusNoContent {
		resp := h.raw(http.MethodDelete, path, body, wantStatus)
		_ = resp.Body.Close()
		return
	}
	h.do(http.MethodDelete, path, body, wantStatus, out)
}

// postError issues POST path with a JSON body, asserts the HTTP status, and
// requires the error envelope (success=false with a non-empty message). It is
// the counterpart to post for the rejection paths (e.g. a locked-workspace 409).
func (h *harness) postError(
	path string,
	body any,
	wantStatus int,
) {
	h.t.Helper()
	h.mutationError(http.MethodPost, path, body, wantStatus)
}

// mutationError issues a mutation with a JSON body, asserts the HTTP status, and
// requires the error envelope (success=false with a non-empty message). It
// returns the message so a caller can assert on the reason as well as the code —
// a refusal the user has to act on is only useful if it says what to do.
func (h *harness) mutationError(
	method string,
	path string,
	body any,
	wantStatus int,
) string {
	h.t.Helper()
	resp := h.raw(method, path, body, wantStatus)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&env))
	require.False(h.t, env.Success, "expected error envelope for %s %s", method, path)
	require.NotEmpty(h.t, env.Error, "error envelope must carry a message")
	return env.Error
}

func (h *harness) do(
	method string,
	path string,
	body any,
	wantStatus int,
	out any,
) {
	h.t.Helper()
	resp := h.raw(method, path, body, wantStatus)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(h.t, json.NewDecoder(resp.Body).Decode(&env))
	require.True(h.t, env.Success, "envelope error for %s %s: %s", method, path, env.Error)
	if out != nil {
		require.NoError(h.t, json.Unmarshal(env.Data, out))
	}
}

func (h *harness) raw(
	method string,
	path string,
	body any,
	wantStatus int,
) *http.Response {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, h.url+path, reader)
	require.NoError(h.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.server.Client().Do(req)
	require.NoError(h.t, err)
	require.Equal(h.t, wantStatus, resp.StatusCode, "%s %s", method, path)
	return resp
}

// dial opens a WebSocket to the given /v0 path and registers cleanup.
func (h *harness) dial(
	path string,
) *websocket.Conn {
	h.t.Helper()
	wsURL := "ws" + h.url[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readUntil blocks reading JSON frames, skipping control and non-JSON frames,
// until match returns true for a decoded frame, and returns that frame.
//
// It carries NO read deadline, by design. The frame's arrival IS the signal, so
// the read simply blocks until the daemon produces it. A deadline here would
// only restate `go test -timeout` — and restate it worse: an expired deadline
// surfaces as a bare "i/o timeout" on some assertion, telling you nothing about
// which frame never came, and it is the construct that goes flaky under load.
//
// A frame that genuinely never arrives is therefore a HANG, resolved by
// `go test -timeout` (the suite's only backstop). That is the better failure:
// the timeout panics with a full goroutine dump in which this read is parked in
// conn.ReadMessage, and the stack immediately above it names the calling test
// and the exact readUntil call site — i.e. it points straight at the frame that
// never came. The only assertion left here is a real one: a read ERROR, meaning
// the connection broke before the awaited frame arrived.
func readUntil(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	for {
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "ws closed before the awaited frame arrived")
		if mt != websocket.TextMessage {
			continue
		}
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil {
			continue
		}
		if match(got) {
			return got
		}
	}
}

// sentinelFile is the file a test dirties in a watched worktree to push a frame
// of its OWN through a Git topic. It is the barrier used to prove NEGATIVES on a
// WS topic without a clock: no fixture ever creates or touches it, so a status
// frame listing it is unambiguously the test's own, and its arrival proves that
// every frame the observed window was ever going to produce has already been
// delivered on that connection.
const sentinelFile = "crowbar-sentinel.txt"

// readTextFrame blocks until the next TEXT frame and returns its raw bytes,
// skipping control frames. Like readUntil it carries no deadline: the frame's
// arrival is the signal. Use it (rather than readUntil) when the test must
// COUNT or inspect every frame, including the ones it does not match on.
func readTextFrame(
	t *testing.T,
	conn *websocket.Conn,
) []byte {
	t.Helper()
	for {
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "ws closed before the awaited frame arrived")
		if mt == websocket.TextMessage {
			return raw
		}
	}
}

// statusHasFile reports whether a decoded GitStatus frame lists name among its
// dirty files. The Git broadcaster filters on wsId but serialises a BARE
// GitStatus (no wsId on the wire), so a frame's dirty-file set is the only
// discriminator a WS test has for which workspace produced it.
func statusHasFile(
	frame map[string]any,
	name string,
) bool {
	files, ok := frame["files"].([]any)
	if !ok {
		return false
	}
	for _, f := range files {
		entry, ok := f.(map[string]any)
		if ok && entry["path"] == name {
			return true
		}
	}
	return false
}

// waitForWorkComplete blocks until wsID's in-flight async mutation has STARTED and
// then FINISHED, returning the final frame. It is the completion barrier for the
// fire-and-forget 202 handlers, whose slow work (provisioning a worktree, running a
// merge) happens in a detached goroutine that no projection drain can observe.
//
// It reads the daemon's OWN "am I busy" overlay: every detached op is bracketed
// BeginWork → EndWork, and EndWork fires only after the work function has returned,
// on success, error and panic alike. Both edges broadcast a WorkspaceDTO, so the
// falling edge is the daemon stating the op is over. Waiting for the RISING edge
// first is what makes the falling one mean anything — a workspace at rest already
// reads working:false, so matching that alone would return before the op began.
//
// Mirrors kit.WaitForWorkComplete for the httptest-based suites in this package.
func waitForWorkComplete(
	t *testing.T,
	conn *websocket.Conn,
	wsID string,
) map[string]any {
	t.Helper()
	isWorking := func(m map[string]any, want bool) bool {
		if m["id"] != wsID {
			return false
		}
		working, ok := m["working"].(bool)
		return ok && working == want
	}
	readUntil(t, conn, func(m map[string]any) bool { return isWorking(m, true) })
	return readUntil(t, conn, func(m map[string]any) bool { return isWorking(m, false) })
}

// gitRepoWithCommit creates a real on-disk git repo with one committed file and
// returns its path. The repo backs a workspace so git/file/watcher flows are
// real rather than mocked.
func gitRepoWithCommit(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	commands := [][]string{
		{"init"},
		{"config", "user.email", "t@t.dev"},
		{"config", "user.name", "t"},
		{"checkout", "-b", "main"},
	}
	for _, args := range commands {
		runGit(t, dir, args...)
	}
	require.NoError(t, writeFile(dir, "README.md", "hello\n"))
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}
