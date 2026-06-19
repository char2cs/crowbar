//go:build integration

package v0_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

type testContainers struct {
	app *app.Container
	eng *engine.Container
}

func newApp(t *testing.T) testContainers {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return testContainers{app: a, eng: eng}
}

func workspaceFixture() dto.WorkspaceDTO {
	return dto.WorkspaceDTO{ID: "w1", RepoID: "r1", ProjectID: "p1"}
}

func TestV0_HubBroadcastReachesWSClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	tc.app.Hub.BroadcastWorkspace(workspaceFixture())

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}

func TestV0_WorkspacesFilter_ProjectId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// The nested WS route binds projectId/repoId as path params, so the client
	// scope is "p1/r1"; the p1/r1/w1 fixture prefix-matches and is delivered.
	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	// This workspace has projectId=p1/repoId=r1 so it passes the prefix filter.
	tc.app.Hub.BroadcastWorkspace(workspaceFixture())

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}

func TestV0_WorkspacesFilter_RepoId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Prefix scoping is hierarchical: a repo-scoped subscription supplies both
	// projectId and repoId (the standalone repoId query is no longer a scope key).
	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	tc.app.Hub.BroadcastWorkspace(workspaceFixture())

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}

// TestContainer_PushProject_RouteByPrefix proves the Projects broadcaster routes
// by hierarchical prefix derived from the :projectId path param: a client scoped
// to "p1" receives only that project's frame; a sibling project's frame is
// filtered out (spec §5).
func TestContainer_PushProject_RouteByPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	r.GET("/v0/projects/:projectId", c.ProjectsHandle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitProjectsRegistered()

	// A sibling project's frame must be filtered out; only p1's arrives.
	tc.app.Hub.BroadcastProject(dto.ProjectDTO{ID: "p2", Name: "skip"})
	tc.app.Hub.BroadcastProject(dto.ProjectDTO{ID: "p1", Name: "keep"})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "p1", got["id"])
	assert.Equal(t, "keep", got["name"])
}

// TestContainer_PushRepo_RouteByPrefix proves the Repos broadcaster routes by
// hierarchical prefix derived from the :projectId path param: a client scoped to
// the project "p1" (prefix "p1") receives every child repo ("p1/r1") but not a
// sibling project's repo ("p2/r1") (spec §5).
func TestContainer_PushRepo_RouteByPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos", c.ReposHandle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitReposRegistered()

	// A sibling project's repo must be filtered out; only p1's child repo arrives.
	tc.app.Hub.BroadcastRepo(dto.RepoDTO{ID: "r1", ProjectID: "p2", Name: "skip"})
	tc.app.Hub.BroadcastRepo(dto.RepoDTO{ID: "r1", ProjectID: "p1", Name: "keep"})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "r1", got["id"])
	assert.Equal(t, "p1", got["projectId"])
	assert.Equal(t, "keep", got["name"])
}

func TestV0_PushLSP_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces/w1/lsp/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitLSPRegistered()

	// An event for a different workspace must be filtered out; the matching one
	// must arrive.
	c.PushLSP(lspdomain.DiagnosticsEvent{WsID: "other", Diagnostics: []lspdomain.Diagnostic{{Message: "skip"}}})
	c.PushLSP(lspdomain.DiagnosticsEvent{WsID: "w1", Diagnostics: []lspdomain.Diagnostic{{Message: "boom"}}})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["wsId"])
	diags, _ := got["diagnostics"].([]any)
	require.Len(t, diags, 1)
}

func TestV0_PushGit_QueryScope_IsolatesWsId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces/A/git/status"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitGitRegistered()

	// A push for workspace B must be filtered out; only A's status arrives.
	tc.app.Hub.BroadcastGit("B", gitdomain.GitStatus{Branch: "branch-B"})
	tc.app.Hub.BroadcastGit("A", gitdomain.GitStatus{Branch: "branch-A"})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got gitdomain.GitStatus
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "branch-A", got.Branch)
}

func TestV0_GitDualServe_PathScope_IsolatesWsId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// The dual-served route scopes by the :wsId PATH param, not a query param.
	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces/A/git/status"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitGitRegistered()

	tc.app.Hub.BroadcastGit("B", gitdomain.GitStatus{Branch: "branch-B"})
	tc.app.Hub.BroadcastGit("A", gitdomain.GitStatus{Branch: "branch-A"})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got gitdomain.GitStatus
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "branch-A", got.Branch)
}

func TestV0_PushFile_ReachesFilteredClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/projects/p1/repos/r1/workspaces/w1/files/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitFilesRegistered()

	tc.app.Hub.BroadcastFile(domain.FileChangeEvent{WsID: "other", Path: "skip.go"})
	tc.app.Hub.BroadcastFile(domain.FileChangeEvent{WsID: "w1", Path: "a.go"})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got domain.FileChangeEvent
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "a.go", got.Path)
}
