//go:build integration

package v0_test

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func gitRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@t.dev"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		require.NoError(t, cmd.Run())
	}
	return dir
}

func dialWS(
	t *testing.T,
	srv *httptest.Server,
	path string,
) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestWatcherLifecycle_FilesSubscriberStartsWatcher proves the full lazy chain:
// subscribing to the Files topic for a real workspace starts the per-workspace
// watcher (WatcherManager.Acquire -> factory -> watch.Watcher.Start), so a real
// filesystem change fans out as a FileChangeEvent over the WS.
func TestWatcherLifecycle_FilesSubscriberStartsWatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	repoPath := gitRepo(t)

	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: repoPath},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWS(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/files/ws")
	c.WaitFilesRegistered()

	// The ticker paces the STIMULUS, it is not the synchronisation.
	//
	// The watcher's inotify registration happens on an app-layer goroutine behind
	// the subscribe hook, and nothing in the API layer can observe the moment the
	// watch is armed. A single write could therefore land before the watch exists
	// and be lost forever. So the write is re-applied until one is picked up. The
	// ASSERTION still blocks on the real signal below — the delivered
	// FileChangeEvent — and never on a duration.
	writer := time.NewTicker(100 * time.Millisecond)
	t.Cleanup(writer.Stop)
	go func() {
		i := 0
		for range writer.C {
			i++
			name := filepath.Join(repoPath, "hello"+strconv.Itoa(i)+".txt")
			_ = os.WriteFile(name, []byte("hi"), 0o644)
		}
	}()

	// Blocking reads, no deadline: the event's arrival IS the signal. A watcher
	// that never starts hangs here until `go test -timeout` fires and names this
	// test, instead of a 10-second guess.
	for {
		mt, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		if mt == websocket.TextMessage && len(msg) > 0 {
			return // a FileChangeEvent arrived: the watcher started lazily on subscribe
		}
	}
}

type fileProbe struct {
	ch chan domain.FileChangeEvent
}

func (fileProbe) PushProject(_ dto.ProjectDTO)                 {}
func (fileProbe) PushRepo(_ dto.RepoDTO)                       {}
func (fileProbe) PushWorkspace(_ dto.WorkspaceDTO)             {}
func (fileProbe) PushThread(_ dto.ThreadDTO)                   {}
func (fileProbe) PushTerminalSession(_ dto.TerminalSessionDTO) {}
func (fileProbe) PushGit(_ string, _ gitdomain.GitStatus)      {}
func (fileProbe) PushAgentChat(_, _, _ string, _ bool)         {}

func (fileProbe) PushAgentChatFolder(_, _, _ string) {}

func (fileProbe) PushAgentChatTerminalWait(_, _ string, _ *dto.AgentTerminalWaitDTO) {}

func (fileProbe) PushAgentChatPromptSettled(_, _, _ string) {}

func (fileProbe) PushAgentChatMessageDelta(_, _, _, _ string) {}

func (fileProbe) PushAgentRunner(_, _, _, _ string) {}

func (p fileProbe) PushFile(
	e domain.FileChangeEvent,
) {
	select {
	case p.ch <- e:
	default:
	}
}

// TestWatcherLifecycle_LSPOnlySubscriberDoesNotStartWatcher proves the two
// refcounts are independent: an LSP-only subscriber for a workspace must NOT
// start the file watcher, so no FileChangeEvent is produced for a real edit.
func TestWatcherLifecycle_LSPOnlySubscriberDoesNotStartWatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	repoPath := gitRepo(t)

	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: repoPath},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	probe := fileProbe{ch: make(chan domain.FileChangeEvent, 1)}
	tc.app.Hub.Register(probe)

	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	_ = dialWS(t, srv, "/v0/projects/p1/repos/r1/workspaces/w1/lsp/ws")
	c.WaitLSPRegistered()

	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "x.txt"), []byte("a"), 0o644))

	// RETAINED CLOCK — the one in this wave, and deliberate.
	//
	// This is a pure negative: it asserts an event is NEVER produced. Unlike the
	// handler negatives (which block on WaitAsync, a real completion signal, and
	// then check exactly), there is nothing here to block on — the assertion is
	// about a watcher that, if the code is correct, does not exist, so it emits no
	// signal of any kind. Waiting is all that is left.
	//
	// The deterministic instrument would be a watcher-refcount probe on
	// realtime.WatcherManager ("how many watchers are live for w1?"), turning this
	// into a synchronous state assertion with no clock at all. That type lives in
	// internal/app/realtime, outside this wave's scope; see the wave report.
	//
	// Note this clock is safe in the direction that matters: a slow machine makes
	// the test MORE conservative, never flaky. It can only ever produce a false
	// PASS (a wrongly-started watcher that was slow to fire), never a false fail.
	select {
	case <-probe.ch:
		t.Fatal("watcher started for an LSP-only subscriber")
	case <-time.After(time.Second):
		// no file event: the watcher stayed dormant, as required
	}
}
