package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// errStore is a sentinel error used to drive the store-failure branches.
var errStore = errors.New("store failure")

// recordingRepoBroadcaster captures each broadcast RepoDTO and signals a channel
// so async completion can be awaited without a sleep.
type recordingRepoBroadcaster struct {
	ch chan dto.RepoDTO
}

func newRecordingRepoBroadcaster() *recordingRepoBroadcaster {
	return &recordingRepoBroadcaster{ch: make(chan dto.RepoDTO, 4)}
}

func (b *recordingRepoBroadcaster) push(
	d dto.RepoDTO,
) {
	b.ch <- d
}

func (b *recordingRepoBroadcaster) await(
	t *testing.T,
) dto.RepoDTO {
	t.Helper()
	select {
	case d := <-b.ch:
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RepoDTO broadcast")
		return dto.RepoDTO{}
	}
}

func statRepoOK(
	_ string,
) (os.FileInfo, error) {
	return nil, nil
}

// newCreateRouter mounts POST .../repos backed by store + broadcaster, with the
// stat seam reporting the path exists so create passes synchronous validation.
func newCreateRouter(
	store repohandlers.Store,
	bc *recordingRepoBroadcaster,
) *gin.Engine {
	r := gin.New()
	h := repohandlers.NewWithDeps(store, nil, nil, bc.push).WithStat(statRepoOK)
	rg := r.Group("/v0/projects/:projectId")
	rg.POST("/repos", h.Create)
	return r
}

func doPost(
	r *gin.Engine,
	target string,
	body any,
) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// TestCreateRepo_Returns202 pins the fail-fast/good-path-async contract: a valid
// create returns 202 with an empty body.
func TestCreateRepo_Returns202(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	rec := doPost(newCreateRouter(&fakeStore{}, bc), "/v0/projects/p1/repos",
		map[string]any{"id": "r1", "name": "alpha", "path": "/tmp/repo"})
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())
	bc.await(t) // drain the async broadcast so the goroutine completes
}

// TestCreateRepo_BroadcastsRepoDTO pins that the background create broadcasts the
// created repo on the Repos channel, with the projectId derived from the path.
func TestCreateRepo_BroadcastsRepoDTO(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	rec := doPost(newCreateRouter(&fakeStore{}, bc), "/v0/projects/p1/repos",
		map[string]any{"id": "r1", "name": "alpha", "path": "/tmp/repo", "defaultBranch": "main"})
	require.Equal(t, http.StatusAccepted, rec.Code)

	got := bc.await(t)
	assert.Equal(t, "r1", got.ID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "alpha", got.Name)
	assert.Empty(t, got.Status)
}

// fakeRepoImporter records the ImportRepo call so the test can assert the
// full-import path runs in the background and returns a repo to broadcast.
type fakeRepoImporter struct {
	gotProjectID string
	gotRepoPath  string
	repo         domain.Repository
	err          error
	called       chan struct{}
}

func newFakeRepoImporter(
	repo domain.Repository,
) *fakeRepoImporter {
	return &fakeRepoImporter{repo: repo, called: make(chan struct{}, 1)}
}

func (f *fakeRepoImporter) ImportRepo(
	_ context.Context,
	projectID string,
	repoPath string,
) (domain.Repository, error) {
	f.gotProjectID = projectID
	f.gotRepoPath = repoPath
	f.called <- struct{}{}
	return f.repo, f.err
}

// TestCreateRepo_RunsFullImport pins §14 Step 3: a valid create runs the full
// repo import in the background (adopting default-branch workspaces + GitHub
// avatar) via the injected RepoImporter, then broadcasts the imported RepoDTO.
func TestCreateRepo_RunsFullImport(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	imp := newFakeRepoImporter(domain.Repository{
		ID:        "r1",
		ProjectID: "p1",
		Name:      "alpha",
		Path:      "/tmp/repo",
	})
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, bc.push).
		WithStat(statRepoOK).
		WithImporter(imp)
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)

	rec := doPost(r, "/v0/projects/p1/repos",
		map[string]any{"name": "alpha", "path": "/tmp/repo"})
	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())

	got := bc.await(t)
	assert.Equal(t, "r1", got.ID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "alpha", got.Name)
	assert.Empty(t, got.Status)

	<-imp.called
	assert.Equal(t, "p1", imp.gotProjectID)
	assert.Equal(t, "/tmp/repo", imp.gotRepoPath)
}

// TestCreateRepo_ValidationFailsSync_4xx pins that a missing path fails the
// synchronous validation with a 4xx before the importer is ever invoked.
func TestCreateRepo_ValidationFailsSync_4xx(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	imp := newFakeRepoImporter(domain.Repository{ID: "r1"})
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, bc.push).
		WithStat(func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }).
		WithImporter(imp)
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)

	rec := doPost(r, "/v0/projects/p1/repos",
		map[string]any{"name": "alpha", "path": "/missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	select {
	case <-imp.called:
		t.Fatal("importer must not run when synchronous validation fails")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestCreateRepo_ImportError_NoBroadcast pins that a failed background import
// broadcasts no RepoDTO (no per-repo LastError sink).
func TestCreateRepo_ImportError_NoBroadcast(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	imp := newFakeRepoImporter(domain.Repository{ID: "r1"})
	imp.err = errStore
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, bc.push).
		WithStat(statRepoOK).
		WithImporter(imp)
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)

	rec := doPost(r, "/v0/projects/p1/repos",
		map[string]any{"name": "alpha", "path": "/tmp/repo"})
	require.Equal(t, http.StatusAccepted, rec.Code)

	<-imp.called
	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast on failed import: %+v", d)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestCreateRepo_PathMissing_4xx pins that a non-existent path fails
// synchronously before any background work is scheduled.
func TestCreateRepo_PathMissing_4xx(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, bc.push).
		WithStat(func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)

	rec := doPost(r, "/v0/projects/p1/repos",
		map[string]any{"name": "alpha", "path": "/missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestCreateRepo_MissingName_4xx pins synchronous name validation.
func TestCreateRepo_MissingName_4xx(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	rec := doPost(newCreateRouter(&fakeStore{}, bc), "/v0/projects/p1/repos",
		map[string]any{"path": "/tmp/repo"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestDeleteRepo_Returns202 pins that a valid delete returns 202 and broadcasts a
// deleted-status RepoDTO tombstone after teardown.
func TestDeleteRepo_Returns202(
	t *testing.T,
) {
	home := t.TempDir()
	var deletedID string
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.DeleteFn = func(_ context.Context, id string) error {
		deletedID = id
		return nil
	}
	bc := newRecordingRepoBroadcaster()
	h := repohandlers.NewWithDeps(store, nil, nil, bc.push).WithIconStorage(
		func() (string, error) { return home, nil },
		nil,
	)
	r := gin.New()
	r.Group("/v0/projects/:projectId/repos/:repoId").DELETE("", h.DeleteRepo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())

	got := bc.await(t)
	assert.Equal(t, "r1", got.ID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "deleted", got.Status)
	assert.Equal(t, "r1", deletedID)
}

// TestCreateRepo_BadJSON_4xx pins synchronous body-shape validation.
func TestCreateRepo_BadJSON_4xx(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, bc.push).WithStat(statRepoOK)
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/projects/p1/repos", bytes.NewReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestCreateRepo_SaveError_NoBroadcast pins that a failed background save
// broadcasts no DTO frame (no per-repo LastError sink).
func TestCreateRepo_SaveError_NoBroadcast(
	t *testing.T,
) {
	store := &fakeStore{}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error {
		return errStore
	}
	bc := newRecordingRepoBroadcaster()
	rec := doPost(newCreateRouter(store, bc), "/v0/projects/p1/repos",
		map[string]any{"id": "r1", "name": "alpha", "path": "/tmp/repo"})
	require.Equal(t, http.StatusAccepted, rec.Code)

	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast on failed save: %+v", d)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestDeleteRepo_FindError_5xx pins that a store lookup error surfaces
// synchronously rather than scheduling background work.
func TestDeleteRepo_FindError_5xx(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	store := &fakeStore{byKeErr: errStore}
	h := repohandlers.NewWithDeps(store, nil, nil, bc.push)
	r := gin.New()
	r.Group("/v0/projects/:projectId/repos/:repoId").DELETE("", h.DeleteRepo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestDeleteRepo_DeleteError_NoBroadcast pins that a failed background delete
// broadcasts no tombstone.
func TestDeleteRepo_DeleteError_NoBroadcast(
	t *testing.T,
) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.DeleteFn = func(_ context.Context, _ string) error {
		return errStore
	}
	bc := newRecordingRepoBroadcaster()
	h := repohandlers.NewWithDeps(store, nil, nil, bc.push).WithIconStorage(
		func() (string, error) { return home, nil },
		nil,
	)
	r := gin.New()
	r.Group("/v0/projects/:projectId/repos/:repoId").DELETE("", h.DeleteRepo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1", http.NoBody)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast on failed delete: %+v", d)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestDeleteRepo_NotFound_4xx pins synchronous existence validation.
func TestDeleteRepo_NotFound_4xx(
	t *testing.T,
) {
	bc := newRecordingRepoBroadcaster()
	store := &fakeStore{byKey: nil}
	h := repohandlers.NewWithDeps(store, nil, nil, bc.push)
	r := gin.New()
	r.Group("/v0/projects/:projectId/repos/:repoId").DELETE("", h.DeleteRepo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/missing", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
