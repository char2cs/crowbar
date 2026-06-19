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
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeReader struct {
	list    []domain.Project
	listErr error
	get     domain.Project
	getErr  error
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

// await blocks until the next broadcast frame or a short deadline, failing the
// test on timeout so a missing async broadcast surfaces deterministically.
func (b *recordingBroadcaster) await(
	t *testing.T,
) dto.ProjectDTO {
	t.Helper()
	select {
	case d := <-b.ch:
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ProjectDTO broadcast")
		return dto.ProjectDTO{}
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
	r := gin.New()
	h := projecthandlers.New(reader, importer, deleter, bc.push).WithStat(statOK)
	rg := r.Group("/v0")
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:projectId", h.Detail)
	rg.DELETE("/projects/:projectId", h.Delete)
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
	rec := do(
		newRouterFull(&fakeReader{}, importer, &fakeDeleter{}, bc),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma","path":"/g"}`,
	)
	require.Equal(t, http.StatusAccepted, rec.Code)

	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast on failed import: %+v", d)
	case <-time.After(100 * time.Millisecond):
	}
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
	rec := do(
		newRouterFull(&fakeReader{get: domain.Project{ID: "p1"}}, &fakeImporter{}, deleter, bc),
		http.MethodDelete,
		"/v0/projects/p1",
		"",
	)
	require.Equal(t, http.StatusAccepted, rec.Code)

	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast on failed delete: %+v", d)
	case <-time.After(100 * time.Millisecond):
	}
}
