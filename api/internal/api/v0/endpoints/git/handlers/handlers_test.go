package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

type writeCall struct {
	method  string
	wsID    string
	args    []string
	boolArg bool
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
	calls       []writeCall
	writeErr    error
}

func (f *fakeGit) record(
	method string,
	wsID string,
	args ...string,
) error {
	f.calls = append(f.calls, writeCall{method: method, wsID: wsID, args: args})
	return f.writeErr
}

func (f *fakeGit) StageFile(
	_ context.Context,
	wsID string,
	filePath string,
	_ time.Time,
) error {
	return f.record("StageFile", wsID, filePath)
}

func (f *fakeGit) StageHunk(
	_ context.Context,
	wsID string,
	filePath string,
	hunkID string,
	_ time.Time,
) error {
	return f.record("StageHunk", wsID, filePath, hunkID)
}

func (f *fakeGit) UnstageFile(
	_ context.Context,
	wsID string,
	filePath string,
	_ time.Time,
) error {
	return f.record("UnstageFile", wsID, filePath)
}

func (f *fakeGit) UnstageHunk(
	_ context.Context,
	wsID string,
	filePath string,
	hunkID string,
	_ time.Time,
) error {
	return f.record("UnstageHunk", wsID, filePath, hunkID)
}

func (f *fakeGit) Discard(
	_ context.Context,
	wsID string,
	filePath string,
	_ time.Time,
) error {
	return f.record("Discard", wsID, filePath)
}

func (f *fakeGit) Commit(
	_ context.Context,
	wsID string,
	subject string,
	body string,
	_ time.Time,
) error {
	return f.record("Commit", wsID, subject, body)
}

func (f *fakeGit) Push(
	_ context.Context,
	wsID string,
	_ time.Time,
) error {
	return f.record("Push", wsID)
}

func (f *fakeGit) Fetch(
	_ context.Context,
	wsID string,
	_ time.Time,
) error {
	return f.record("Fetch", wsID)
}

func (f *fakeGit) Pull(
	_ context.Context,
	wsID string,
	mode string,
	_ time.Time,
) error {
	return f.record("Pull", wsID, mode)
}

func (f *fakeGit) CreateBranch(
	_ context.Context,
	wsID string,
	name string,
	source string,
	switchTo bool,
	_ time.Time,
) error {
	f.calls = append(f.calls, writeCall{
		method:  "CreateBranch",
		wsID:    wsID,
		args:    []string{name, source},
		boolArg: switchTo,
	})
	return f.writeErr
}

func (f *fakeGit) RenameBranch(
	_ context.Context,
	wsID string,
	oldName string,
	newName string,
	_ time.Time,
) error {
	return f.record("RenameBranch", wsID, oldName, newName)
}

func (f *fakeGit) DeleteBranch(
	_ context.Context,
	wsID string,
	name string,
	_ time.Time,
) error {
	return f.record("DeleteBranch", wsID, name)
}

func (f *fakeGit) SwitchBranch(
	_ context.Context,
	wsID string,
	name string,
	_ time.Time,
) error {
	return f.record("SwitchBranch", wsID, name)
}

func (f *fakeGit) StashPush(
	_ context.Context,
	wsID string,
	message string,
	_ time.Time,
) error {
	return f.record("StashPush", wsID, message)
}

func (f *fakeGit) StashApply(
	_ context.Context,
	wsID string,
	id string,
	_ time.Time,
) error {
	return f.record("StashApply", wsID, id)
}

func (f *fakeGit) StashPop(
	_ context.Context,
	wsID string,
	id string,
	_ time.Time,
) error {
	return f.record("StashPop", wsID, id)
}

func (f *fakeGit) StashDrop(
	_ context.Context,
	wsID string,
	id string,
	_ time.Time,
) error {
	return f.record("StashDrop", wsID, id)
}

func (f *fakeGit) Reset(
	_ context.Context,
	wsID string,
	mode string,
	commit string,
	_ time.Time,
) error {
	return f.record("Reset", wsID, mode, commit)
}

func (f *fakeGit) Merge(
	_ context.Context,
	wsID string,
	branch string,
	_ time.Time,
) error {
	return f.record("Merge", wsID, branch)
}

func (f *fakeGit) Rebase(
	_ context.Context,
	wsID string,
	onto string,
	_ time.Time,
) error {
	return f.record("Rebase", wsID, onto)
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
	r.UseRawPath = true
	r.UnescapePathValues = true
	h := githandlers.New(git, func() time.Time { return time.Unix(0, 0).UTC() })
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/git/status", h.Status)
	rg.GET("/workspaces/:wsId/git/log", h.Log)
	rg.GET("/workspaces/:wsId/git/diff", h.Diff)
	rg.GET("/workspaces/:wsId/git/branches", h.Branches)
	rg.GET("/workspaces/:wsId/git/stashes", h.Stashes)
	rg.POST("/workspaces/:wsId/git/stage", h.Stage)
	rg.POST("/workspaces/:wsId/git/unstage", h.Unstage)
	rg.POST("/workspaces/:wsId/git/discard", h.Discard)
	rg.POST("/workspaces/:wsId/git/commit", h.Commit)
	rg.POST("/workspaces/:wsId/git/push", h.Push)
	rg.POST("/workspaces/:wsId/git/pull", h.Pull)
	rg.POST("/workspaces/:wsId/git/fetch", h.Fetch)
	rg.POST("/workspaces/:wsId/git/branches", h.CreateBranch)
	rg.PATCH("/workspaces/:wsId/git/branches/:branch", h.RenameBranch)
	rg.DELETE("/workspaces/:wsId/git/branches/:branch", h.DeleteBranch)
	rg.POST("/workspaces/:wsId/git/checkout", h.Checkout)
	rg.POST("/workspaces/:wsId/git/stash", h.StashPush)
	rg.POST("/workspaces/:wsId/git/stash/:id", h.StashRestore)
	rg.DELETE("/workspaces/:wsId/git/stash/:id", h.StashDrop)
	rg.POST("/workspaces/:wsId/git/reset", h.Reset)
	rg.POST("/workspaces/:wsId/git/merge", h.Merge)
	rg.POST("/workspaces/:wsId/git/rebase", h.Rebase)
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

func send(
	r *gin.Engine,
	method string,
	target string,
	body string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}
