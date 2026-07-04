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
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
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
	list     []domain.Workspace
	listErr  error
	get      domain.Workspace
	getErr   error
	gotID    string
	synced   domain.Workspace
	syncErr  error
	gotSync  string
	syncDone chan struct{}
	elig     map[string]workspace.MergeEligibility
	gotElig  [][]domain.Workspace
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
	if f.syncDone != nil {
		close(f.syncDone)
	}
	return f.synced, f.syncErr
}

func (f *fakeReader) MergeEligibilityFor(
	_ context.Context,
	ws domain.Workspace,
	siblings []domain.Workspace,
) workspace.MergeEligibility {
	f.gotElig = append(f.gotElig, siblings)
	return f.elig[ws.ID]
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
	createDone   chan struct{}
	deleteDone   chan struct{}
	mergeDone    chan struct{}
	reparentDone chan struct{}
}

func (f *fakeHierarchy) CreateChild(
	_ context.Context,
	in worktree.CreateChildInput,
) (domain.Workspace, error) {
	f.gotCreate = in
	if f.createDone != nil {
		close(f.createDone)
	}
	return f.created, f.createErr
}

func (f *fakeHierarchy) MergeIntoParent(
	_ context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (worktree.MergeResult, error) {
	f.gotMergeID = childID
	f.gotStrategy = strategy
	if f.mergeDone != nil {
		close(f.mergeDone)
	}
	return f.mergeResult, f.mergeErr
}

func (f *fakeHierarchy) Reparent(
	_ context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	f.gotReparent = childID
	f.gotNewParent = newParentID
	if f.reparentDone != nil {
		close(f.reparentDone)
	}
	return f.reparented, f.reparentErr
}

func (f *fakeHierarchy) RebaseOntoParent(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeHierarchy) DeleteCascade(
	_ context.Context,
	rootID string,
) error {
	f.gotDeleteID = rootID
	if f.deleteDone != nil {
		close(f.deleteDone)
	}
	return f.deleteErr
}

func (f *fakeHierarchy) RetryProvision(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeHierarchy) DetachHolder(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
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

type fakeLastErrors struct {
	gotID  string
	gotMsg string
	called chan struct{}
}

func (f *fakeLastErrors) SetLastError(
	_ context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	f.gotID = id
	f.gotMsg = message
	if f.called != nil {
		f.called <- struct{}{}
	}
	return domain.Workspace{ID: id, LastError: message}, nil
}

// fakeWork satisfies workspacehandlers.WorkSignal for tests that don't assert
// the working overlay.
type fakeWork struct{}

func (fakeWork) BeginWork(_ context.Context, _ string) {}
func (fakeWork) EndWork(_ context.Context, _ string)   {}
func (fakeWork) IsWorking(_ string) bool               { return false }

func newRouter(
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
) *gin.Engine {
	r := gin.New()
	h := workspacehandlers.New(reader, hierarchy, repos, &fakeLastErrors{}, fakeWork{})
	// Mount under the hierarchical repo-scoped prefix so the handlers read
	// :projectId/:repoId/:wsId from the path, mirroring the production router.
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.GET("/workspaces", h.List)
	rg.GET("/workspaces/:wsId", h.Detail)
	rg.POST("/workspaces", h.Create)
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rg.POST("/workspaces/:wsId/sync", h.Sync)
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	rg.POST("/workspaces/:wsId/retry-provision", h.RetryProvision)
	rg.POST("/workspaces/:wsId/detach-holder", h.DetachHolder)
	return r
}

// waitClosed blocks until done is closed, failing the test on a deadline so a
// background goroutine that never runs surfaces as a clear failure instead of a
// silent hang (no time.Sleep polling).
func waitClosed(
	t *testing.T,
	done chan struct{},
) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background work to run")
	}
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
