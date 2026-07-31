package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
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
	list                 []domain.Workspace
	listErr              error
	listInRepo           []domain.Workspace
	listInRepoErr        error
	gotListInRepoProject string
	gotListInRepoRepo    string
	get                  domain.Workspace
	getErr               error
	gotID                string
	synced               domain.Workspace
	syncErr              error
	gotSync              string
	syncDone             chan struct{}
	elig                 map[string]workspace.MergeEligibility
	gotElig              [][]domain.Workspace
}

func (f *fakeReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return f.list, f.listErr
}

// ListInRepo records the project/repo it was asked to scope to and returns the
// configured listInRepo/listInRepoErr — the fake performs no filtering itself
// (that responsibility now lives in the real ListInRepo implementation, tested
// at the repo/usecase layer); it exists here purely to let handler tests assert
// List/Detail forward the right :projectId/:repoId to the reader.
func (f *fakeReader) ListInRepo(
	_ context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	f.gotListInRepoProject = projectID
	f.gotListInRepoRepo = repoID
	return f.listInRepo, f.listInRepoErr
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
	renamed      domain.Workspace
	renameErr    error
	gotRenameID  string
	gotRenameTo  string
	gotImport    worktree.ImportInput
	importErr    error
	importDone   chan struct{}
	gotRebaseID  string
	rebaseErr    error
}

func (f *fakeHierarchy) CreateFromImport(
	_ context.Context,
	in worktree.ImportInput,
) error {
	f.gotImport = in
	if f.importDone != nil {
		close(f.importDone)
	}
	return f.importErr
}

func (f *fakeHierarchy) RenameBranch(
	_ context.Context,
	wsID string,
	newBranch string,
) (domain.Workspace, error) {
	f.gotRenameID, f.gotRenameTo = wsID, newBranch
	if f.renameErr != nil {
		return domain.Workspace{}, f.renameErr
	}
	return f.renamed, nil
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
	childID string,
) (domain.Workspace, error) {
	f.gotRebaseID = childID
	return domain.Workspace{}, f.rebaseErr
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
func (fakeWork) WorkingFor(_ string) bool              { return false }

func newRouter(
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
) *gin.Engine {
	r, _, _ := newRouterWithPlacer(reader, hierarchy, repos, nil)
	return r
}

// fakePlacer records the sidebar placement the PATCH handler asked for and
// returns canned results. `folders` is the repo's folder list the create path
// resolves a folderId against.
type fakePlacer struct {
	placed       domain.Workspace
	shifted      []domain.Folder
	folders      []domain.Folder
	listErr      error
	err          error
	gotWS        string
	gotIn        folder.PlaceInput
	calls        int
	nextSlot     int
	nextSlotErr  error
	gotContainer string
}

func (f *fakePlacer) PlaceWorkspace(
	_ context.Context,
	_ string,
	_ string,
	wsID string,
	in folder.PlaceInput,
) (domain.Workspace, []domain.Folder, error) {
	f.calls++
	f.gotWS = wsID
	f.gotIn = in
	return f.placed, f.shifted, f.err
}

func (f *fakePlacer) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
) ([]domain.Folder, error) {
	return f.folders, f.listErr
}

func (f *fakePlacer) NextSlot(
	_ context.Context,
	_ string,
	_ string,
	container string,
) (int, error) {
	f.gotContainer = container
	return f.nextSlot, f.nextSlotErr
}

// newRouterWithPlacer additionally wires the sidebar placer and captures the
// folder frames the PATCH handler fans out.
func newRouterWithPlacer(
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
	placer workspacehandlers.Placer,
) (*gin.Engine, *fakePlacer, *[]dto.FolderDTO) {
	frames := &[]dto.FolderDTO{}
	r := gin.New()
	h := workspacehandlers.New(reader, hierarchy, repos, &fakeLastErrors{}, fakeWork{}).
		WithPlacer(placer, func(d dto.FolderDTO) { *frames = append(*frames, d) })
	// Mount under the hierarchical repo-scoped prefix so the handlers read
	// :projectId/:repoId/:wsId from the path, mirroring the production router.
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.GET("/workspaces", h.List)
	rg.GET("/workspaces/:wsId", h.Detail)
	rg.POST("/workspaces", h.Create)
	rg.POST("/workspaces/import", h.Import)
	rg.PATCH("/workspaces/:wsId", h.Patch)
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rg.POST("/workspaces/:wsId/sync", h.Sync)
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	rg.POST("/workspaces/:wsId/retry-provision", h.RetryProvision)
	rg.POST("/workspaces/:wsId/detach-holder", h.DetachHolder)
	concrete, _ := placer.(*fakePlacer)
	return r, concrete, frames
}

// waitClosed blocks until done is closed. The close IS the "the background work
// ran" signal, so a plain receive is the whole synchronisation: it returns the
// instant the goroutine gets there, however loaded the machine. Background work
// that never runs hangs here until `go test -timeout` fires and dumps the
// goroutines — a real failure with a real stack, rather than a one-second guess
// that goes red on a busy CI box.
func waitClosed(
	t *testing.T,
	done chan struct{},
) {
	t.Helper()
	<-done
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
