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
	"github.com/char2cs/crowbar/api/internal/app/hub"
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

	conn := dialWS(t, srv, "/v0/ws/files?wsId=w1")
	c.WaitFilesRegistered()

	// The watcher starts asynchronously on the subscribe hook; re-write on a
	// ticker until the first event lands so the test never races the goroutine
	// that adds the inotify watch (no fixed sleep).
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

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
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

func (fileProbe) PushWorkspace(_ domain.Workspace)        {}
func (fileProbe) PushChat(_ hub.ChatStatusEvent)          {}
func (fileProbe) PushGit(_ string, _ gitdomain.GitStatus) {}

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

	_ = dialWS(t, srv, "/v0/ws/lsp?wsId=w1")
	c.WaitLSPRegistered()

	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "x.txt"), []byte("a"), 0o644))

	select {
	case <-probe.ch:
		t.Fatal("watcher started for an LSP-only subscriber")
	case <-time.After(time.Second):
		// no file event: the watcher stayed dormant, as required
	}
}
