package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeGit struct {
	status      gitdomain.GitStatus
	statusErr   error
	diff        []gitdomain.FileDiff
	diffStaged  bool
	diffErr     error
	commitDiff  gitdomain.MultiFileDiff
	gotCommit   string
	commitErr   error
	commits     []gitdomain.Commit
	gotLimit    int
	gotSkip     int
	logErr      error
	branches    []gitdomain.Branch
	branchesErr error
	stashes     []gitdomain.Stash
	stashesErr  error
}

func (f *fakeGit) Status(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return f.status, f.statusErr
}

func (f *fakeGit) Diff(
	_ context.Context,
	_ string,
	staged bool,
) ([]gitdomain.FileDiff, error) {
	f.diffStaged = staged
	return f.diff, f.diffErr
}

func (f *fakeGit) CommitDiff(
	_ context.Context,
	_ string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	f.gotCommit = sha
	return f.commitDiff, f.commitErr
}

func (f *fakeGit) Log(
	_ context.Context,
	_ string,
	limit int,
	skip int,
) ([]gitdomain.Commit, error) {
	f.gotLimit = limit
	f.gotSkip = skip
	return f.commits, f.logErr
}

func (f *fakeGit) Branches(
	_ context.Context,
	_ string,
) ([]gitdomain.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeGit) Stashes(
	_ context.Context,
	_ string,
) ([]gitdomain.Stash, error) {
	return f.stashes, f.stashesErr
}

func newRouter(
	git githandlers.Git,
) *gin.Engine {
	r := gin.New()
	h := githandlers.New(git)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/git/status", h.Status)
	rg.GET("/workspaces/:wsId/git/log", h.Log)
	rg.GET("/workspaces/:wsId/git/diff", h.Diff)
	rg.GET("/workspaces/:wsId/git/branches", h.Branches)
	rg.GET("/workspaces/:wsId/git/stashes", h.Stashes)
	return r
}

func do(
	r *gin.Engine,
	target string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}
