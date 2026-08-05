package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	folderhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/folders/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// fakeUsecase records each call and returns canned results, so the handler's
// HTTP contract can be pinned without a store.
type fakeUsecase struct {
	created   domain.Folder
	shifted   []domain.Folder
	renamed   domain.Folder
	moved     domain.Folder
	list      []domain.Folder
	err       error
	gotCreate folder.CreateInput
	gotMove   folder.MoveInput
	gotRename string
	gotDelete string
	renames   int
	moves     int
}

func (f *fakeUsecase) Create(
	_ context.Context,
	in folder.CreateInput,
) (domain.Folder, []domain.Folder, error) {
	f.gotCreate = in
	return f.created, f.shifted, f.err
}

func (f *fakeUsecase) Rename(
	_ context.Context,
	_ string,
	name string,
) (domain.Folder, error) {
	f.renames++
	f.gotRename = name
	return f.renamed, f.err
}

func (f *fakeUsecase) Move(
	_ context.Context,
	_ string,
	in folder.MoveInput,
) (domain.Folder, []domain.Folder, error) {
	f.moves++
	f.gotMove = in
	return f.moved, f.shifted, f.err
}

func (f *fakeUsecase) Delete(
	_ context.Context,
	id string,
) ([]domain.Folder, error) {
	f.gotDelete = id
	return f.shifted, f.err
}

func (f *fakeUsecase) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
) ([]domain.Folder, error) {
	return f.list, f.err
}

func newRouter(
	uc folderhandlers.Usecase,
	frames *[]dto.FolderDTO,
) *gin.Engine {
	h := folderhandlers.New(uc, func(d dto.FolderDTO) { *frames = append(*frames, d) })
	r := gin.New()
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.GET("/folders", h.List)
	rg.POST("/folders", h.Create)
	rg.PATCH("/folders/:folderId", h.Patch)
	rg.DELETE("/folders/:folderId", h.Delete)
	return r
}

func do(
	r *gin.Engine,
	method string,
	target string,
	body any,
) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

const base = "/v0/projects/p1/repos/r1/folders"

func TestList_ReturnsOrderedDTOs(t *testing.T) {
	uc := &fakeUsecase{list: []domain.Folder{
		{ID: "b", Order: 1, Name: "b"},
		{ID: "a", Order: 0, Name: "a"},
	}}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodGet, base, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Data []dto.FolderDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 2)
	assert.Equal(t, "a", env.Data[0].ID, "the list handler serves sidebar order")
}

// The URL scope is authoritative: a POST to one repo must never create a folder
// in another, so the body carries no project or repo at all.
func TestCreate_TakesTheScopeFromTheURL(t *testing.T) {
	uc := &fakeUsecase{created: domain.Folder{ID: "f1", ProjectID: "p1", RepoID: "r1", Name: "spikes"}}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPost, base,
		map[string]any{"name": "spikes", "parentId": "w1", "projectId": "evil", "repoId": "evil"})

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "p1", uc.gotCreate.ProjectID)
	assert.Equal(t, "r1", uc.gotCreate.RepoID)
	assert.Equal(t, "w1", uc.gotCreate.ParentID)
	require.Len(t, frames, 1)
	assert.Equal(t, "f1", frames[0].ID)
}

// A create densifies the level, so the rows it shifted have to reach the client
// too — otherwise their orders stay stale until the next reconnect.
func TestCreate_BroadcastsTheCollateral(t *testing.T) {
	uc := &fakeUsecase{
		created: domain.Folder{ID: "f1"},
		shifted: []domain.Folder{{ID: "f0", Order: 0}},
	}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPost, base, map[string]any{"name": "spikes"})

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, frames, 2)
	assert.Equal(t, "f0", frames[0].ID)
	assert.Equal(t, "f1", frames[1].ID)
}

func TestCreate_SurfacesTheUsecaseRefusal(t *testing.T) {
	uc := &fakeUsecase{err: folder.ErrFolderNameRequired}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPost, base, map[string]any{"name": " "})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, frames, "a refused create broadcasts nothing")
}

func TestCreate_MalformedBody_400(t *testing.T) {
	uc := &fakeUsecase{}
	var frames []dto.FolderDTO
	r := newRouter(uc, &frames)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, base, bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// A drag that renames AND moves must land as one broadcast, not two half-states.
func TestPatch_RenameThenMoveBroadcastsOnce(t *testing.T) {
	uc := &fakeUsecase{
		renamed: domain.Folder{ID: "f1", Name: "new"},
		moved:   domain.Folder{ID: "f1", Name: "new", ParentID: "w1", Order: 2},
	}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPatch, base+"/f1",
		map[string]any{"name": "new", "parentId": "w1", "order": 2})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, uc.renames)
	assert.Equal(t, 1, uc.moves)
	assert.Equal(t, "new", uc.gotRename)
	require.NotNil(t, uc.gotMove.ParentID)
	assert.Equal(t, "w1", *uc.gotMove.ParentID)
	require.NotNil(t, uc.gotMove.Order)
	assert.Equal(t, 2, *uc.gotMove.Order)
	require.Len(t, frames, 1, "one gesture, one frame")
	assert.Equal(t, 2, frames[0].Order)
}

// A PATCH that reorders within one parent carries no parentId, and the nil must
// reach the usecase as "leave it where it is" rather than "move it to the root".
func TestPatch_OrderOnlyLeavesTheParentNil(t *testing.T) {
	uc := &fakeUsecase{moved: domain.Folder{ID: "f1", Order: 1}}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPatch, base+"/f1", map[string]any{"order": 1})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, uc.renames)
	assert.Nil(t, uc.gotMove.ParentID)
	require.NotNil(t, uc.gotMove.Order)
	assert.Equal(t, 1, *uc.gotMove.Order)
}

// An explicit empty parentId is a MOVE TO THE ROOT, and must be distinguishable
// from an absent one.
func TestPatch_EmptyParentMeansTheRepoRoot(t *testing.T) {
	uc := &fakeUsecase{moved: domain.Folder{ID: "f1"}}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPatch, base+"/f1", map[string]any{"parentId": ""})

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, uc.gotMove.ParentID)
	assert.Equal(t, "", *uc.gotMove.ParentID)
}

func TestPatch_CycleIsAConflict(t *testing.T) {
	uc := &fakeUsecase{err: folder.ErrFolderCycle}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPatch, base+"/f1", map[string]any{"parentId": "f2"})

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, frames)
}

func TestPatch_NotFoundIs404(t *testing.T) {
	uc := &fakeUsecase{err: apperr.ErrNotFound}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPatch, base+"/f1", map[string]any{"order": 0})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPatch_MalformedBody_400(t *testing.T) {
	uc := &fakeUsecase{}
	var frames []dto.FolderDTO
	r := newRouter(uc, &frames)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, base+"/f1", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, uc.moves)
}

// The tombstone is what makes the client cache drop the entity; the reparented
// rows are what stop its children vanishing with it.
func TestDelete_BroadcastsReparentedRowsThenTheTombstone(t *testing.T) {
	uc := &fakeUsecase{shifted: []domain.Folder{{ID: "child", ParentID: ""}}}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodDelete, base+"/f1", nil)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "f1", uc.gotDelete)
	require.Len(t, frames, 2)
	assert.Equal(t, "child", frames[0].ID)
	assert.Empty(t, frames[0].Status, "a reparented row is live, not a tombstone")
	assert.Equal(t, "f1", frames[1].ID)
	assert.Equal(t, "deleted", frames[1].Status)
	assert.Equal(t, "p1", frames[1].ProjectID)
	assert.Equal(t, "r1", frames[1].RepoID)
}

func TestDelete_NotFoundIs404(t *testing.T) {
	uc := &fakeUsecase{err: apperr.ErrNotFound}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodDelete, base+"/f1", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, frames, "a failed delete must not tombstone an entity that still exists")
}

// A nil broadcast must degrade to a no-op rather than panic, matching every
// other handler wired without a hub.
func TestNew_NilBroadcastDegradesToNoop(t *testing.T) {
	h := folderhandlers.New(&fakeUsecase{created: domain.Folder{ID: "f1"}}, nil)
	r := gin.New()
	r.POST("/v0/projects/:projectId/repos/:repoId/folders", h.Create)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, base, bytes.NewReader([]byte(`{"name":"spikes"}`)))
	req.Header.Set("Content-Type", "application/json")
	assert.NotPanics(t, func() { r.ServeHTTP(rec, req) })
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestList_SurfacesAStoreError(t *testing.T) {
	uc := &fakeUsecase{err: errors.New("boom")}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodGet, base, nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A failed rename must stop before the placement runs, or a refused PATCH would
// still half-apply.
func TestPatch_FailedRenameSkipsTheMove(t *testing.T) {
	uc := &fakeUsecase{err: folder.ErrFolderNameRequired}
	var frames []dto.FolderDTO
	rec := do(newRouter(uc, &frames), http.MethodPatch, base+"/f1",
		map[string]any{"name": " ", "order": 0})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, uc.renames)
	assert.Zero(t, uc.moves, "the placement must not run after a refused rename")
	assert.Empty(t, frames)
}
