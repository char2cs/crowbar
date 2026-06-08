package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeReader struct {
	list    []domain.Workspace
	listErr error
	get     domain.Workspace
	getErr  error
	gotID   string
	synced  domain.Workspace
	syncErr error
	gotSync string
}

func (f *fakeReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return f.list, f.listErr
}

func (f *fakeReader) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	f.gotID = id
	return f.get, f.getErr
}

func (f *fakeReader) SyncWorkingTreeState(
	_ context.Context,
	id string,
	_ time.Time,
) (domain.Workspace, error) {
	f.gotSync = id
	return f.synced, f.syncErr
}

type fakeHierarchy struct {
	created      domain.Workspace
	createErr    error
	gotCreate    worktree.CreateChildInput
	mergeResult  worktree.MergeResult
	mergeErr     error
	gotMergeID   string
	gotStrategy  gitdomain.MergeStrategy
	reparented   domain.Workspace
	reparentErr  error
	gotReparent  string
	gotNewParent string
	deleteErr    error
	gotDeleteID  string
}

func (f *fakeHierarchy) CreateChild(
	_ context.Context,
	in worktree.CreateChildInput,
) (domain.Workspace, error) {
	f.gotCreate = in
	return f.created, f.createErr
}

func (f *fakeHierarchy) MergeIntoParent(
	_ context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (worktree.MergeResult, error) {
	f.gotMergeID = childID
	f.gotStrategy = strategy
	return f.mergeResult, f.mergeErr
}

func (f *fakeHierarchy) Reparent(
	_ context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	f.gotReparent = childID
	f.gotNewParent = newParentID
	return f.reparented, f.reparentErr
}

func (f *fakeHierarchy) DeleteCascade(
	_ context.Context,
	rootID string,
) error {
	f.gotDeleteID = rootID
	return f.deleteErr
}

type fakeRepos struct {
	repo *domain.Repository
	err  error
}

func (f *fakeRepos) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	return f.repo, f.err
}

func newRouter(
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
) *gin.Engine {
	r := gin.New()
	h := workspacehandlers.New(reader, hierarchy, repos)
	rg := r.Group("/v0")
	rg.GET("/workspaces", h.List)
	rg.GET("/workspaces/:wsId", h.Detail)
	rg.POST("/workspaces", h.Create)
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rg.POST("/workspaces/:wsId/sync", h.Sync)
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	return r
}

func do(
	r *gin.Engine,
	method string,
	target string,
	body string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, http.NoBody)
	}
	r.ServeHTTP(rec, req)
	return rec
}
