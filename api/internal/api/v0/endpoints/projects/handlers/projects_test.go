package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeReader struct {
	list       []domain.Project
	listErr    error
	get        domain.Project
	getErr     error
	reordered  domain.Project
	reorderErr error
	// reorderTo records the index the last Reorder call asked for, so a test can
	// assert the handler passed the body's order through untouched.
	reorderTo int
	// updated is what Update answers with, and updatedWith records the partial it
	// was handed — what a rename or an icon change is asserted through.
	updated     domain.Project
	updateErr   error
	updatedWith *project.Update
}

func (f *fakeReader) Update(
	_ context.Context,
	_ string,
	in project.Update,
) (domain.Project, error) {
	f.updatedWith = &in
	if f.updateErr != nil {
		return domain.Project{}, f.updateErr
	}
	return f.updated, nil
}

func (f *fakeReader) List(
	_ context.Context,
) ([]domain.Project, error) {
	return f.list, f.listErr
}

func (f *fakeReader) Get(
	_ context.Context,
	_ string,
) (domain.Project, error) {
	return f.get, f.getErr
}

func (f *fakeReader) Reorder(
	_ context.Context,
	_ string,
	order int,
) (domain.Project, error) {
	f.reorderTo = order
	return f.reordered, f.reorderErr
}

type fakeImporter struct {
	project domain.Project
	err     error
	gotName string
	gotPath string
}

func (f *fakeImporter) Import(
	_ context.Context,
	name string,
	path string,
) (domain.Project, error) {
	f.gotName = name
	f.gotPath = path
	return f.project, f.err
}

func (f *fakeImporter) Create(
	_ context.Context,
	name string,
	path string,
) (domain.Project, error) {
	f.gotName = name
	f.gotPath = path
	return f.project, f.err
}

type fakeDeleter struct {
	err   error
	gotID string
}

func (f *fakeDeleter) Delete(
	_ context.Context,
	id string,
) error {
	f.gotID = id
	return f.err
}

// recordingBroadcaster captures each broadcast ProjectDTO and signals a channel
// so async completion can be awaited without a sleep.
type recordingBroadcaster struct {
	ch chan dto.ProjectDTO
}

func newRecordingBroadcaster() *recordingBroadcaster {
	return &recordingBroadcaster{ch: make(chan dto.ProjectDTO, 4)}
}

func (b *recordingBroadcaster) push(
	d dto.ProjectDTO,
) {
	b.ch <- d
}

// await blocks until the background op broadcasts. The frame's arrival IS the
// signal, so a plain receive is the whole synchronisation; a broadcast that
// never comes hangs until `go test -timeout` fires and names this test, instead
// of a two-second guess that reddens under load.
func (b *recordingBroadcaster) await(
	t *testing.T,
) dto.ProjectDTO {
	t.Helper()
	return <-b.ch
}

// assertNoBroadcast pins a NEGATIVE: the background op broadcast nothing.
//
// That claim is only sound once the producing goroutine is provably dead, which
// no sleep can establish — it merely widens the window a slow goroutine can hide
// in. h.WaitAsync() blocks until every detached op has fully returned, after
// which the broadcaster has no writer left and this non-blocking check is exact.
func assertNoBroadcast(
	t *testing.T,
	h *projecthandlers.Handlers,
	bc *recordingBroadcaster,
) {
	t.Helper()
	h.WaitAsync()
	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast from a failed background op: %+v", d)
	default:
	}
}

// statOK is a stat seam that always reports the path exists.
func statOK(
	_ string,
) (os.FileInfo, error) {
	return nil, nil
}

func newRouter(
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
) *gin.Engine {
	return newRouterFull(reader, importer, &fakeDeleter{}, newRecordingBroadcaster())
}

func newRouterFull(
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	bc *recordingBroadcaster,
) *gin.Engine {
	r, _ := newRouterWithHandlers(reader, importer, deleter, bc)
	return r
}

// newRouterWithHandlers is newRouterFull, also returning the Handlers so a test
// can block on WaitAsync — the real completion signal for the detached
// background op.
func newRouterWithHandlers(
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	bc *recordingBroadcaster,
) (*gin.Engine, *projecthandlers.Handlers) {
	r := gin.New()
	h := projecthandlers.New(reader, importer, deleter, bc.push).WithStat(statOK)
	rg := r.Group("/v0")
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:projectId", h.Detail)
	rg.PATCH("/projects/:projectId", h.Patch)
	rg.DELETE("/projects/:projectId", h.Delete)
	return r, h
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
		req = httptest.NewRequest(method, target, nil)
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestListSuccess(
	t *testing.T,
) {
	reader := &fakeReader{
		list: []domain.Project{
			{ID: "p1", Name: "alpha", Path: "/a", LastActivity: time.Unix(1, 0).UTC()},
		},
	}
	rec := do(newRouter(reader, &fakeImporter{}), http.MethodGet, "/v0/projects", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.Len(t, body.Data, 1)
	assert.Equal(t, "p1", body.Data[0].ID)
	assert.Equal(t, "alpha", body.Data[0].Name)
}

func TestListError(
	t *testing.T,
) {
	reader := &fakeReader{listErr: errors.New("db down")}
	rec := do(newRouter(reader, &fakeImporter{}), http.MethodGet, "/v0/projects", "")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.NotEmpty(t, body.Error)
}

func TestDetailSuccess(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Project{ID: "p9", Name: "beta", Path: "/b"}}
	rec := do(newRouter(reader, &fakeImporter{}), http.MethodGet, "/v0/projects/p9", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "p9", body.Data.ID)
}

func TestDetailError(
	t *testing.T,
) {
	reader := &fakeReader{getErr: errors.New("missing")}
	rec := do(newRouter(reader, &fakeImporter{}), http.MethodGet, "/v0/projects/nope", "")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestImportProject_Returns202 pins the fail-fast/good-path-async contract: a
// valid import returns 202 with an empty body (the created id arrives later on
// the WebSocket stream, never in the HTTP response).
func TestImportProject_Returns202(
	t *testing.T,
) {
	importer := &fakeImporter{project: domain.Project{ID: "new-id"}}
	bc := newRecordingBroadcaster()
	rec := do(
		newRouterFull(&fakeReader{}, importer, &fakeDeleter{}, bc),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma","path":"/g"}`,
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())
	bc.await(t) // drain the async broadcast so the goroutine completes
}

// TestImportProject_BroadcastsProjectDTO pins that the background import
// broadcasts the created project on the Projects channel.
func TestImportProject_BroadcastsProjectDTO(
	t *testing.T,
) {
	importer := &fakeImporter{project: domain.Project{ID: "new-id", Name: "gamma", Path: "/g"}}
	bc := newRecordingBroadcaster()
	rec := do(
		newRouterFull(&fakeReader{}, importer, &fakeDeleter{}, bc),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma","path":"/g"}`,
	)
	require.Equal(t, http.StatusAccepted, rec.Code)

	got := bc.await(t)
	assert.Equal(t, "new-id", got.ID)
	assert.Equal(t, "gamma", got.Name)
	assert.Empty(t, got.Status)
	assert.Equal(t, "gamma", importer.gotName)
	assert.Equal(t, "/g", importer.gotPath)
}

// TestImportProject_QuickUsesCreate pins that the quick flag routes to the
// lightweight Create usecase.
func TestImportProject_QuickUsesCreate(
	t *testing.T,
) {
	importer := &fakeImporter{project: domain.Project{ID: "q1"}}
	bc := newRecordingBroadcaster()
	rec := do(
		newRouterFull(&fakeReader{}, importer, &fakeDeleter{}, bc),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma","path":"/g","quick":true}`,
	)
	require.Equal(t, http.StatusAccepted, rec.Code)
	got := bc.await(t)
	assert.Equal(t, "q1", got.ID)
}

func TestImportBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeImporter{}),
		http.MethodPost,
		"/v0/projects",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestImportMissingName(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeImporter{}),
		http.MethodPost,
		"/v0/projects",
		`{"path":"/g"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestImportMissingPath(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeImporter{}),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestImportProject_PathMissing_4xx pins that a non-existent path fails
// synchronously with a 4xx before any background work is scheduled.
func TestImportProject_PathMissing_4xx(
	t *testing.T,
) {
	r := gin.New()
	bc := newRecordingBroadcaster()
	h := projecthandlers.New(&fakeReader{}, &fakeImporter{}, &fakeDeleter{}, bc.push).
		WithStat(func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
	r.POST("/v0/projects", h.Import)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/projects", strings.NewReader(`{"name":"g","path":"/missing"}`))
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestImportProject_UsecaseError_NoBroadcast pins that a failed background
// import produces no DTO frame (no per-project LastError sink).
func TestImportProject_UsecaseError_NoBroadcast(
	t *testing.T,
) {
	importer := &fakeImporter{err: errors.New("boom")}
	bc := newRecordingBroadcaster()
	r, h := newRouterWithHandlers(&fakeReader{}, importer, &fakeDeleter{}, bc)
	rec := do(r, http.MethodPost, "/v0/projects", `{"name":"gamma","path":"/g"}`)
	require.Equal(t, http.StatusAccepted, rec.Code)

	assertNoBroadcast(t, h, bc)
}

// TestDeleteProject_Returns202_TombstoneDTO pins that a valid delete returns 202
// and broadcasts a deleted-status ProjectDTO tombstone after teardown.
func TestDeleteProject_Returns202_TombstoneDTO(
	t *testing.T,
) {
	deleter := &fakeDeleter{}
	bc := newRecordingBroadcaster()
	rec := do(
		newRouterFull(&fakeReader{get: domain.Project{ID: "p1"}}, &fakeImporter{}, deleter, bc),
		http.MethodDelete,
		"/v0/projects/p1",
		"",
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())

	got := bc.await(t)
	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "deleted", got.Status)
	assert.Equal(t, "p1", deleter.gotID)
}

func TestDeleteNotFound(
	t *testing.T,
) {
	bc := newRecordingBroadcaster()
	rec := do(
		newRouterFull(&fakeReader{getErr: apperr.ErrNotFound}, &fakeImporter{}, &fakeDeleter{}, bc),
		http.MethodDelete,
		"/v0/projects/nope",
		"",
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.NotEmpty(t, body.Error)
}

// TestDeleteProject_UsecaseError_NoBroadcast pins that a failed background
// delete broadcasts no tombstone.
func TestDeleteProject_UsecaseError_NoBroadcast(
	t *testing.T,
) {
	deleter := &fakeDeleter{err: errors.New("boom")}
	bc := newRecordingBroadcaster()
	r, h := newRouterWithHandlers(
		&fakeReader{get: domain.Project{ID: "p1"}}, &fakeImporter{}, deleter, bc,
	)
	rec := do(r, http.MethodDelete, "/v0/projects/p1", "")
	require.Equal(t, http.StatusAccepted, rec.Code)

	assertNoBroadcast(t, h, bc)
}

// The reorder densifies the whole list, so every project has to reach the
// client: told only about the row that was dragged, a client holds stale orders
// for the rest until the next reconnect.
func TestPatch_ReordersAndBroadcastsTheWholeList(t *testing.T) {
	reader := &fakeReader{
		reordered: domain.Project{ID: "p2", Order: 0},
		list: []domain.Project{
			{ID: "p2", Order: 0},
			{ID: "p1", Order: 1},
		},
	}
	bc := newRecordingBroadcaster()
	r := newRouterFull(reader, &fakeImporter{}, &fakeDeleter{}, bc)

	rec := do(r, http.MethodPatch, "/v0/projects/p2", `{"order":0}`)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 0, reader.reorderTo)
	first := <-bc.ch
	second := <-bc.ch
	assert.Equal(t, "p2", first.ID)
	assert.Equal(t, "p1", second.ID)
	assert.Equal(t, 1, second.Order, "the shifted row's new order reaches the client")
}

func TestPatch_MissingOrderIs400(t *testing.T) {
	reader := &fakeReader{}
	r := newRouter(reader, &fakeImporter{})

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPatch_MalformedBodyIs400(t *testing.T) {
	r := newRouter(&fakeReader{}, &fakeImporter{})

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPatch_UnknownProjectIs404(t *testing.T) {
	reader := &fakeReader{reorderErr: apperr.ErrNotFound}
	bc := newRecordingBroadcaster()
	r := newRouterFull(reader, &fakeImporter{}, &fakeDeleter{}, bc)

	rec := do(r, http.MethodPatch, "/v0/projects/missing", `{"order":0}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, bc.ch, "a refused reorder broadcasts nothing")
}

// The reorder already committed; a list that then fails must not turn a
// successful write into an error response. The client re-reads on reconnect.
func TestPatch_ListFailureAfterTheWriteStillAnswers204(t *testing.T) {
	reader := &fakeReader{
		reordered: domain.Project{ID: "p1"},
		listErr:   errors.New("read model down"),
	}
	bc := newRecordingBroadcaster()
	r := newRouterFull(reader, &fakeImporter{}, &fakeDeleter{}, bc)

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{"order":0}`)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, bc.ch)
}

// The sidebar's inline rename, which PATCH grew alongside the reorder it used to
// be the only reason for. Both are single store writes, so both answer 204 and
// deliver the change on the WS stream rather than in the response body.
func TestPatchRenamesAProject(t *testing.T) {
	reader := &fakeReader{updated: domain.Project{ID: "p1", Name: "harbour"}}
	bc := newRecordingBroadcaster()
	r := newRouterFull(reader, &fakeImporter{}, &fakeDeleter{}, bc)

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{"name":"  harbour  "}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if reader.updatedWith == nil || reader.updatedWith.Name == nil {
		t.Fatal("the handler never passed a name through")
	}
	// Trimmed at the edge, so the store never holds a name with the whitespace
	// an inline editor makes it far too easy to leave behind.
	if *reader.updatedWith.Name != "harbour" {
		t.Errorf("name = %q, want %q", *reader.updatedWith.Name, "harbour")
	}
	// A rename shifts nobody, so it delivers the one row it changed — not the
	// whole list a reorder's densify has to re-broadcast.
	if got := bc.await(t).ID; got != "p1" {
		t.Errorf("broadcast project = %q, want p1", got)
	}
	select {
	case extra := <-bc.ch:
		t.Errorf("a rename broadcast a second frame: %+v", extra)
	default:
	}
}

func TestPatchRejectsABlankName(t *testing.T) {
	reader := &fakeReader{}
	r := newRouter(reader, &fakeImporter{})

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{"name":"   "}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH = %d, want 400", rec.Code)
	}
	if reader.updatedWith != nil {
		t.Error("a blank name must never reach the store")
	}
}

func TestPatchStillRequiresSomethingToDo(t *testing.T) {
	r := newRouter(&fakeReader{}, &fakeImporter{})
	if rec := do(r, http.MethodPatch, "/v0/projects/p1", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH = %d, want 400", rec.Code)
	}
}

func TestPatchSurfacesAFailedRename(t *testing.T) {
	// The store refused. Without propagating, the caller gets a 204 and the
	// sidebar shows the new name until the next WS frame puts the old one back.
	reader := &fakeReader{updateErr: apperr.ErrNotFound}
	bc := newRecordingBroadcaster()
	r := newRouterFull(reader, &fakeImporter{}, &fakeDeleter{}, bc)

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{"name":"harbour"}`)

	if rec.Code < 400 {
		t.Fatalf("PATCH = %d, want a 4xx/5xx", rec.Code)
	}
	select {
	case frame := <-bc.ch:
		t.Errorf("a refused rename broadcast anyway: %+v", frame)
	default:
	}
}

func TestPatchRenamesAndReordersInOneRequest(t *testing.T) {
	// A drag can rename nothing and reorder, or (from the inline editor) rename
	// only — but the endpoint accepts both at once, and the reorder's densify is
	// what re-delivers every row. The rename must NOT also broadcast on its own
	// or clients apply the same frame either side of a list that moved.
	reader := &fakeReader{
		updated: domain.Project{ID: "p1", Name: "harbour"},
		list:    []domain.Project{{ID: "p1", Name: "harbour"}},
	}
	bc := newRecordingBroadcaster()
	r := newRouterFull(reader, &fakeImporter{}, &fakeDeleter{}, bc)

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{"name":"harbour","order":0}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if reader.updatedWith == nil || reader.updatedWith.Name == nil {
		t.Fatal("the rename half never reached the store")
	}
	if reader.reorderTo != 0 {
		t.Errorf("reorder index = %d, want 0", reader.reorderTo)
	}
	// Exactly one frame: the densify's, not the rename's as well.
	bc.await(t)
	select {
	case extra := <-bc.ch:
		t.Errorf("a paired rename+reorder broadcast twice: %+v", extra)
	default:
	}
}
