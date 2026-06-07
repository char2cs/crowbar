package git_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
