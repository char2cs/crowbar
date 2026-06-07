package git_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubGit struct{}

func (stubGit) Status(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (stubGit) Diff(
	_ context.Context,
	_ string,
	_ bool,
) ([]gitdomain.FileDiff, error) {
	return nil, nil
}

func (stubGit) CommitDiff(
	_ context.Context,
	_ string,
	_ string,
) (gitdomain.MultiFileDiff, error) {
	return gitdomain.MultiFileDiff{}, nil
}

func (stubGit) Log(
	_ context.Context,
	_ string,
	_ int,
	_ int,
) ([]gitdomain.Commit, error) {
	return nil, nil
}

func (stubGit) Branches(
	_ context.Context,
	_ string,
) ([]gitdomain.Branch, error) {
	return nil, nil
}

func (stubGit) Stashes(
	_ context.Context,
	_ string,
) ([]gitdomain.Stash, error) {
	return nil, nil
}

func (stubGit) StageFile(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) StageHunk(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) UnstageFile(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) UnstageHunk(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Discard(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Commit(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Push(
	_ context.Context,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Fetch(
	_ context.Context,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Pull(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) CreateBranch(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ bool,
	_ time.Time,
) error {
	return nil
}

func (stubGit) RenameBranch(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) DeleteBranch(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) SwitchBranch(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) StashPush(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) StashApply(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) StashPop(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) StashDrop(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Reset(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Merge(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) Rebase(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) ConflictedFiles(
	_ context.Context,
	_ string,
) ([]string, error) {
	return nil, nil
}

func (stubGit) ConflictHunks(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.ConflictHunk, error) {
	return nil, nil
}

func (stubGit) ResolveHunk(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ gitdomain.ConflictResolution,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) OperationContinue(
	_ context.Context,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubGit) OperationAbort(
	_ context.Context,
	_ string,
	_ time.Time,
) error {
	return nil
}

func dualServe(
	rest gin.HandlerFunc,
	ws gin.HandlerFunc,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if websocket.IsWebSocketUpgrade(c.Request) {
			ws(c)
			return
		}
		rest(c)
	}
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	var wsHit bool
	git.Register(
		r.Group("/v0"),
		stubGit{},
		func(_ *gin.Context) { wsHit = true },
		dualServe,
	)

	paths := []string{
		"/v0/workspaces/abc/git/status",
		"/v0/workspaces/abc/git/log",
		"/v0/workspaces/abc/git/diff",
		"/v0/workspaces/abc/git/branches",
		"/v0/workspaces/abc/git/stashes",
	}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, p)
	}
	assert.False(t, wsHit)
}

func TestRegisterMountsWriteRoutes(
	t *testing.T,
) {
	r := gin.New()
	git.Register(
		r.Group("/v0"),
		stubGit{},
		func(_ *gin.Context) {},
		dualServe,
	)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/workspaces/abc/git/stage"},
		{http.MethodPost, "/v0/workspaces/abc/git/unstage"},
		{http.MethodPost, "/v0/workspaces/abc/git/discard"},
		{http.MethodPost, "/v0/workspaces/abc/git/commit"},
		{http.MethodPost, "/v0/workspaces/abc/git/push"},
		{http.MethodPost, "/v0/workspaces/abc/git/pull"},
		{http.MethodPost, "/v0/workspaces/abc/git/fetch"},
		{http.MethodPost, "/v0/workspaces/abc/git/branches"},
		{http.MethodPatch, "/v0/workspaces/abc/git/branches/main"},
		{http.MethodDelete, "/v0/workspaces/abc/git/branches/main"},
		{http.MethodPost, "/v0/workspaces/abc/git/checkout"},
		{http.MethodPost, "/v0/workspaces/abc/git/stash"},
		{http.MethodPost, "/v0/workspaces/abc/git/stash/0"},
		{http.MethodDelete, "/v0/workspaces/abc/git/stash/0"},
		{http.MethodPost, "/v0/workspaces/abc/git/reset"},
		{http.MethodPost, "/v0/workspaces/abc/git/merge"},
		{http.MethodPost, "/v0/workspaces/abc/git/rebase"},
		{http.MethodGet, "/v0/workspaces/abc/git/conflicts"},
		{http.MethodPost, "/v0/workspaces/abc/git/conflicts/resolve"},
		{http.MethodPost, "/v0/workspaces/abc/git/operation/continue"},
		{http.MethodPost, "/v0/workspaces/abc/git/operation/abort"},
	}
	for _, rt := range routes {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(rt.method, rt.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, rt.path)
	}
}

func TestStatusPlainGETServesREST(
	t *testing.T,
) {
	r := gin.New()
	var wsHit bool
	git.Register(
		r.Group("/v0"),
		stubGit{},
		func(_ *gin.Context) { wsHit = true },
		dualServe,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/abc/git/status", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, wsHit)
	assert.Contains(t, rec.Body.String(), `"success":true`)
}

func TestStatusUpgradeServesWS(
	t *testing.T,
) {
	r := gin.New()
	var wsHit bool
	git.Register(
		r.Group("/v0"),
		stubGit{},
		func(c *gin.Context) {
			wsHit = true
			c.Status(http.StatusSwitchingProtocols)
		},
		dualServe,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/abc/git/status", http.NoBody)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(rec, req)

	assert.True(t, wsHit)
}
