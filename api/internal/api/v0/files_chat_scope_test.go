package v0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// This file proves the files stream's chat-scoped mount end to end over the
// real v0 Container (chatScopeEnv and placementEnv, git_chat_scope_test.go).
// filesDef carries no snapshot, so every assertion reads a live push.

const workspaceFilesRoute = "/v0/projects/p1/repos/r1/workspaces/"

func fileChange(
	wsID string,
	path string,
) domain.FileChangeEvent {
	return domain.FileChangeEvent{Type: domain.FileChangeModified, WsID: wsID, Path: path}
}

// TestFilesFanout_OnePushReachesEveryChatHoldingTheWorktree: three chats over
// one worktree all receive one push; chat-z's first frame is its own.
func TestFilesFanout_OnePushReachesEveryChatHoldingTheWorktree(t *testing.T) {
	c, srv := chatScopeEnv(t)

	owner := dialWSAt(t, srv, "/v0/chats/chat-a/files/ws")
	sibling := dialWSAt(t, srv, "/v0/chats/chat-b/files/ws")
	underFolder := dialWSAt(t, srv, "/v0/chats/chat-c/files/ws")
	unrelated := dialWSAt(t, srv, "/v0/chats/chat-z/files/ws")
	c.files.WaitNRegistered(4)

	c.PushFile(fileChange("ws-a", "a.go"))
	c.PushFile(fileChange("ws-z", "z.go"))

	assert.Equal(t, "a.go", readJSON(t, owner)["path"])
	assert.Equal(t, "a.go", readJSON(t, sibling)["path"],
		"a sibling chat on the same worktree shares its file tree")
	assert.Equal(t, "a.go", readJSON(t, underFolder)["path"],
		"a chat filed under a folder still resolves through it to the worktree")
	assert.Equal(t, "z.go", readJSON(t, unrelated)["path"],
		"chat-z holds ws-z: the ws-a frame must never have reached it")
}

// TestFilesCoexistence_TheWorkspaceScopedRouteIsGone: the retired repo-scoped
// mount refuses the upgrade; the wsId filter stays for the home mount.
func TestFilesCoexistence_TheWorkspaceScopedRouteIsGone(t *testing.T) {
	_, srv := chatScopeEnv(t)

	url := "ws" + srv.URL[len("http"):] + workspaceFilesRoute + "ws-a/files/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err, "the old workspace-scoped files mount must no longer upgrade")
}

// TestFilesDef_AChatWithNoWorktreeMatchesNothing: an unbound chat id matches
// nothing, while a client naming only a wsId is scoped by that alone.
func TestFilesDef_AChatWithNoWorktreeMatchesNothing(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Usecases.Worktree = &rowsWorktreeResolver{
		rows:       fanoutChatRows{{ID: "orphan", Type: domain.ChatTypeChat}},
		workspaces: a.Repositories.Workspace,
	}
	viaChat, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaChat.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/v0/chats/orphan/files/ws", nil)
	viaChat.Params = gin.Params{{Key: "chatId", Value: "orphan"}}
	viaHome, _ := gin.CreateTestContext(httptest.NewRecorder())
	viaHome.Request = httptest.NewRequestWithContext(t.Context(), "GET", "/v0/projects/p1/home/files/ws", nil)
	viaHome.Params = gin.Params{{Key: "wsId", Value: "ws-a"}}

	orphan, _ := ws.BuildPredicate(viaChat, filesDef(a))
	home, _ := ws.BuildPredicate(viaHome, filesDef(a))

	assert.False(t, orphan(fileChange("ws-a", "a.go")))
	assert.False(t, orphan(fileChange("", "a.go")))
	assert.True(t, home(fileChange("ws-a", "a.go")))
	assert.False(t, home(fileChange("ws-z", "z.go")))
}

// readPaths streams every frame's path from conn until the test ends.
func readPaths(
	t *testing.T,
	conn *websocket.Conn,
) <-chan string {
	t.Helper()
	paths := make(chan string, 256)
	go func() {
		defer close(paths)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var evt domain.FileChangeEvent
			if json.Unmarshal(msg, &evt) == nil {
				paths <- evt.Path
			}
		}
	}()
	return paths
}

// TestFilesBinding_FollowsAChatToItsNewWorktree: after a real placement write
// moves the chat onto ws-b, the SAME connection receives ws-b changes and no
// longer ws-a's. Files has no snapshot to signal the rebind, so the test
// probes ws-b until one arrives.
func TestFilesBinding_FollowsAChatToItsNewWorktree(t *testing.T) {
	c, srv, a := placementEnv(t)
	createRealChat(t, a, "chat-x", "", "owner-a")
	paths := readPaths(t, dialWSAt(t, srv, "/v0/chats/chat-x/files/ws"))
	c.files.WaitNRegistered(1)

	c.PushFile(fileChange("ws-a", "on-a.go"))
	assert.Equal(t, "on-a.go", <-paths)

	_, err := a.Repositories.AgentChat.SetWorkspace(context.Background(), "chat-x", "ws-b")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		c.PushFile(fileChange("ws-b", "probe.go"))
		select {
		case p := <-paths:
			return p == "probe.go"
		default:
			return false
		}
	}, wsReadBound, 5*time.Millisecond)

	c.PushFile(fileChange("ws-a", "stale-a.go"))
	c.PushFile(fileChange("ws-b", "on-b.go"))
	for p := range paths {
		if p == "probe.go" {
			continue
		}
		assert.Equal(t, "on-b.go", p, "the old worktree's changes no longer reach the moved chat")
		return
	}
	t.Fatal("connection closed before the ws-b change arrived")
}

// TestRegression_FilesForkAfterConnectReachesEveryChatOnTheWorktree: a chat
// forked onto a worktree after a sibling's stream opened, and the sibling
// itself, both receive the next change.
func TestRegression_FilesForkAfterConnectReachesEveryChatOnTheWorktree(t *testing.T) {
	c, srv, a := placementEnv(t)
	established := dialWSAt(t, srv, "/v0/chats/owner-a/files/ws")
	c.files.WaitNRegistered(1)

	createRealChat(t, a, "chat-new", "", "owner-a")
	newcomer := dialWSAt(t, srv, "/v0/chats/chat-new/files/ws")
	c.files.WaitNRegistered(1)

	c.PushFile(fileChange("ws-a", "after-the-fork.go"))

	assert.Equal(t, "after-the-fork.go", readJSON(t, newcomer)["path"])
	assert.Equal(t, "after-the-fork.go", readJSON(t, established)["path"])
}

// TestRegression_TheFileWatcherFollowsAChatToItsNewWorktree runs the REAL
// watcher: the refcount that starts it must move with the chat's binding, or
// nothing ever produces ws-b's events for a chat moved there after connect.
func TestRegression_TheFileWatcherFollowsAChatToItsNewWorktree(t *testing.T) {
	c, srv, a := placementEnvAt(t, initCleanGitRepo(t))
	createRealChat(t, a, "chat-x", "", "owner-a")
	paths := readPaths(t, dialWSAt(t, srv, "/v0/chats/chat-x/files/ws"))
	c.files.WaitNRegistered(1)
	dirA := worktreeOf(t, a, "ws-a")
	dirB := worktreeOf(t, a, "ws-b")

	awaitRealChange(t, paths, dirA, "a-probe")

	_, err := a.Repositories.AgentChat.SetWorkspace(context.Background(), "chat-x", "ws-b")
	require.NoError(t, err)
	awaitRealChange(t, paths, dirB, "b-probe")

	require.NoError(t, os.WriteFile(filepath.Join(dirA, "a-stale.txt"), []byte("x"), 0o600))
	awaitRealChange(t, paths, dirB, "b-after")
}

func worktreeOf(t *testing.T, a *app.Container, wsID string) string {
	t.Helper()
	w, err := a.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err)
	return w.WorktreePath
}

// awaitRealChange writes a fresh prefix-named file into dir until one of them
// reaches paths: the watcher arms asynchronously, so a single write could land
// before it is watching. The tick outlasts the watcher's trailing debounce,
// which writes any closer together would keep resetting. Any a-stale frame
// seen on the way fails the test.
func awaitRealChange(t *testing.T, paths <-chan string, dir, prefix string) {
	t.Helper()
	written := 0
	require.Eventually(t, func() bool {
		written++
		name := fmt.Sprintf("%s-%d.txt", prefix, written)
		if os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600) != nil {
			return false
		}
		for {
			select {
			case p := <-paths:
				base := filepath.Base(p)
				assert.NotEqual(t, "a-stale.txt", base, "the old worktree's change reached the moved chat")
				if strings.HasPrefix(base, prefix+"-") {
					return true
				}
			default:
				return false
			}
		}
	}, wsReadBound, 250*time.Millisecond)
}
