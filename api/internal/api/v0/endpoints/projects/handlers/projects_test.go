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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/api/libs"
	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestMain(m *testing.M) {
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

func (f *fakeReader) Update(_ context.Context, _ string, in project.Update) (domain.Project, error) {
	f.updatedWith = &in
	if f.updateErr != nil {
		return domain.Project{}, f.updateErr
	}
	return f.updated, nil
}

func (f *fakeReader) List(_ context.Context) ([]domain.Project, error) {
	return f.list, f.listErr
}

func (f *fakeReader) Get(_ context.Context, _ string) (domain.Project, error) {
	return f.get, f.getErr
}

func (f *fakeReader) Reorder(_ context.Context, _ string, order int) (domain.Project, error) {
	f.reorderTo = order
	return f.reordered, f.reorderErr
}

type fakeImporter struct {
	project domain.Project
	err     error
	gotID   string
	gotName string
	gotPath string
}

func (f *fakeImporter) Create(_ context.Context, id, name, path string) (domain.Project, error) {
	f.gotID, f.gotName, f.gotPath = id, name, path
	return f.project, f.err
}

type fakeDeleter struct {
	err      error
	beginErr error
	gotID    string
	hold     chan struct{} // when set, Delete waits for it
}

func (f *fakeDeleter) BeginDelete(_ context.Context, id string, _ domain.DeleteConsent) (domain.Project, error) {
	if f.beginErr != nil {
		return domain.Project{}, f.beginErr
	}
	if f.gotID == id && f.err == nil {
		return domain.Project{}, problem.ErrNotFound
	}
	return domain.Project{ID: id, Name: "demo", Deleting: true}, nil
}

func (f *fakeDeleter) Delete(ctx context.Context, id string, _ domain.DeleteConsent) error {
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
		}
	}
	f.gotID = id
	return f.err
}

// recordedOps is the real operation service over an in-memory store, with
// every saved row on frames.
type recordedOps struct {
	*operation.Service
	frames chan domain.Operation
}

func newRecordedOps(t *testing.T) *recordedOps {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	opStore, err := storesqlite.NewFromDB[domain.Operation, string](db)
	require.NoError(t, err)
	frames := make(chan domain.Operation, 64)
	return &recordedOps{Service: operation.New(opStore, func(op domain.Operation) { frames <- op }, time.Now), frames: frames}
}

// settled decodes the operation a 202 answered with and reads frames until it
// has settled.
func (o *recordedOps) settled(t *testing.T, rec *httptest.ResponseRecorder) domain.Operation {
	t.Helper()
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	deadline := time.After(5 * time.Second)
	for {
		select {
		case op := <-o.frames:
			if op.ID == env.Data.Operation.ID && (op.State == domain.OpSucceeded || op.State == domain.OpFailed) {
				return op
			}
		case <-deadline:
			t.Fatalf("operation %s never settled", env.Data.Operation.ID)
		}
	}
}

// statOK is a stat seam that always reports the path exists.
func statOK(_ string) (os.FileInfo, error) {
	return nil, nil
}

func newRouter(reader projecthandlers.ListGetter, importer projecthandlers.Importer) *gin.Engine {
	return newRouterWithOps(reader, importer, &fakeDeleter{}, nil)
}

func newRouterWithOps(
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	ops projecthandlers.Operations,
) *gin.Engine {
	r := gin.New()
	h := projecthandlers.New(reader, importer, deleter, ops, unusedHome).WithStat(statOK)
	rg := r.Group("/v0")
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:projectId", h.Detail)
	rg.PATCH("/projects/:projectId", h.Patch)
	rg.DELETE("/projects/:projectId", h.Delete)
	return r
}

func do(r *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	return doKeyed(r, method, target, body, "")
}

func doKeyed(r *gin.Engine, method, target, body, key string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	if key != "" {
		req.Header.Set(libs.IdempotencyKeyHeader, key)
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestListSuccess(t *testing.T) {
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

func TestListError(t *testing.T) {
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

func TestDetailSuccess(t *testing.T) {
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

func TestDetailError(t *testing.T) {
	reader := &fakeReader{getErr: errors.New("missing")}
	rec := do(newRouter(reader, &fakeImporter{}), http.MethodGet, "/v0/projects/nope", "")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// The create answers with its operation, naming the project under the id it
// minted; the work creates it under that id.
func TestCreate_AnswersWithTheOperation(t *testing.T) {
	importer := &fakeImporter{project: domain.Project{ID: "new-id", Name: "gamma", Path: "/g"}}
	ops := newRecordedOps(t)
	rec := do(newRouterWithOps(&fakeReader{}, importer, &fakeDeleter{}, ops), http.MethodPost, "/v0/projects", `{"name":"gamma","path":"/g"}`)

	op := ops.settled(t, rec)
	assert.Equal(t, domain.OpProjectCreate, op.Kind)
	assert.Equal(t, "Add gamma", op.Label)
	assert.Equal(t, domain.OpSucceeded, op.State)
	require.Len(t, op.Items, 1)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityProject, ID: importer.gotID}, op.Items[0].Ref)
	assert.Equal(t, op.Subject, op.Items[0].Ref)
	assert.Equal(t, importer.gotID, op.ProjectID)
	assert.Equal(t, "gamma", importer.gotName)
	assert.Equal(t, "/g", importer.gotPath)
}

func TestImportBadJSON(t *testing.T) {
	rec := do(newRouter(&fakeReader{}, &fakeImporter{}), http.MethodPost, "/v0/projects", `{not-json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestImportMissingName(t *testing.T) {
	rec := do(newRouter(&fakeReader{}, &fakeImporter{}), http.MethodPost, "/v0/projects", `{"path":"/g"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestImportMissingPath(t *testing.T) {
	rec := do(newRouter(&fakeReader{}, &fakeImporter{}), http.MethodPost, "/v0/projects", `{"name":"gamma"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestImportProject_PathMissing_4xx pins that a non-existent path fails
// synchronously with a 4xx before any background work is scheduled.
func TestImportProject_PathMissing_4xx(t *testing.T) {
	r := gin.New()
	h := projecthandlers.New(&fakeReader{}, &fakeImporter{}, &fakeDeleter{}, nil, unusedHome).
		WithStat(func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
	r.POST("/v0/projects", h.Import)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/projects", strings.NewReader(`{"name":"g","path":"/missing"}`))
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// A create that fails says why on its operation.
func TestCreate_AFailureIsTheOperations(t *testing.T) {
	importer := &fakeImporter{err: errors.New("boom")}
	ops := newRecordedOps(t)
	rec := do(newRouterWithOps(&fakeReader{}, importer, &fakeDeleter{}, ops), http.MethodPost, "/v0/projects", `{"name":"gamma","path":"/g"}`)

	op := ops.settled(t, rec)
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Contains(t, op.Error.Message, "boom")
	assert.Equal(t, domain.OpFailed, op.Items[0].State)
}

// A retry under the same key is the same request: it answers with the first
// operation and imports nothing again.
func TestCreate_ARetryAnswersWithTheFirstOperation(t *testing.T) {
	importer := &fakeImporter{project: domain.Project{ID: "new-id", Name: "gamma", Path: "/g"}}
	ops := newRecordedOps(t)
	r := newRouterWithOps(&fakeReader{}, importer, &fakeDeleter{}, ops)
	key := uuid.NewString()

	first := ops.settled(t, doKeyed(r, http.MethodPost, "/v0/projects", `{"name":"gamma","path":"/g"}`, key))
	importer.gotID = ""
	retry := doKeyed(r, http.MethodPost, "/v0/projects", `{"name":"gamma","path":"/g"}`, key)

	require.Equal(t, http.StatusAccepted, retry.Code, retry.Body.String())
	assert.Contains(t, retry.Body.String(), first.ID)
	assert.Empty(t, importer.gotID, "the retry imported nothing")
}

// A retry of a delete that already finished answers with that operation, not
// the 404 a fresh lookup of the gone project would give.
func TestDelete_ARetryAfterItFinishedAnswersWithTheOperation(t *testing.T) {
	ops := newRecordedOps(t)
	r := newRouterWithOps(&fakeReader{}, &fakeImporter{}, &fakeDeleter{}, ops)
	key := uuid.NewString()

	first := ops.settled(t, doKeyed(r, http.MethodDelete, "/v0/projects/p1", "", key))
	retry := doKeyed(r, http.MethodDelete, "/v0/projects/p1", "", key)

	require.Equal(t, http.StatusAccepted, retry.Code, retry.Body.String())
	assert.Contains(t, retry.Body.String(), first.ID)
}

// A key is one request: reusing a delete's key for another project is a
// conflict, not an answer with the first project's operation.
func TestDelete_AKeyReusedForAnotherProjectIsAConflict(t *testing.T) {
	ops := newRecordedOps(t)
	r := newRouterWithOps(&fakeReader{}, &fakeImporter{}, &fakeDeleter{}, ops)
	key := uuid.NewString()
	ops.settled(t, doKeyed(r, http.MethodDelete, "/v0/projects/p1", "", key))

	rec := doKeyed(r, http.MethodDelete, "/v0/projects/p2", "", key)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "operation_id_conflict")
}

// One teardown runs per project: a second delete under another key, while the
// first runs, is refused.
func TestDelete_ASecondDeleteWhileOneRunsIsRefused(t *testing.T) {
	deleter := &fakeDeleter{hold: make(chan struct{})}
	ops := newRecordedOps(t)
	r := newRouterWithOps(&fakeReader{}, &fakeImporter{}, deleter, ops)

	first := doKeyed(r, http.MethodDelete, "/v0/projects/p1", "", uuid.NewString())
	second := doKeyed(r, http.MethodDelete, "/v0/projects/p1", "", uuid.NewString())
	close(deleter.hold)
	ops.settled(t, first)

	assert.Equal(t, http.StatusConflict, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), "operation_running")
}

func TestDelete_AnswersWithTheOperation(t *testing.T) {
	deleter := &fakeDeleter{}
	ops := newRecordedOps(t)
	rec := do(newRouterWithOps(&fakeReader{}, &fakeImporter{}, deleter, ops), http.MethodDelete, "/v0/projects/p1", "")

	op := ops.settled(t, rec)
	assert.Equal(t, domain.OpProjectDelete, op.Kind)
	assert.Equal(t, "Remove demo", op.Label)
	assert.Equal(t, domain.OpSucceeded, op.State)
	assert.Equal(t, []domain.OperationItem{{
		Ref: domain.EntityRef{Kind: domain.EntityProject, ID: "p1"}, Label: "demo", State: domain.OpSucceeded,
	}}, op.Items)
	assert.Equal(t, "p1", deleter.gotID)
}

func TestDeleteNotFound(t *testing.T) {
	deleter := &fakeDeleter{beginErr: problem.ErrNotFound}
	r := newRouterWithOps(&fakeReader{}, &fakeImporter{}, deleter, newRecordedOps(t))
	rec := do(r, http.MethodDelete, "/v0/projects/nope", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), `"success":false`)
}

// A delete that stops fails its operation with the reason.
func TestDelete_AFailureIsTheOperations(t *testing.T) {
	ops := newRecordedOps(t)
	r := newRouterWithOps(&fakeReader{}, &fakeImporter{}, &fakeDeleter{err: errors.New("boom")}, ops)

	op := ops.settled(t, do(r, http.MethodDelete, "/v0/projects/p1", ""))
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Items[0].Error)
	assert.Contains(t, op.Items[0].Error.Message, "boom")
}

// The reorder passes the body's index to the store and answers 204.
func TestPatch_ReordersTheProject(t *testing.T) {
	reader := &fakeReader{reordered: domain.Project{ID: "p2", Order: 1}}
	r := newRouter(reader, &fakeImporter{})

	rec := do(r, http.MethodPatch, "/v0/projects/p2", `{"order":1}`)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 1, reader.reorderTo)
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
	reader := &fakeReader{reorderErr: problem.ErrNotFound}
	r := newRouterWithOps(reader, &fakeImporter{}, &fakeDeleter{}, nil)

	rec := do(r, http.MethodPatch, "/v0/projects/missing", `{"order":0}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// The sidebar's inline rename, which PATCH grew alongside the reorder it used to
// be the only reason for. Both are single store writes, so both answer 204 and
// deliver the change on the WS stream rather than in the response body.
func TestPatchRenamesAProject(t *testing.T) {
	reader := &fakeReader{updated: domain.Project{ID: "p1", Name: "harbour"}}
	r := newRouterWithOps(reader, &fakeImporter{}, &fakeDeleter{}, nil)

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
	reader := &fakeReader{updateErr: problem.ErrNotFound}
	r := newRouterWithOps(reader, &fakeImporter{}, &fakeDeleter{}, nil)

	rec := do(r, http.MethodPatch, "/v0/projects/p1", `{"name":"harbour"}`)

	if rec.Code < 400 {
		t.Fatalf("PATCH = %d, want a 4xx/5xx", rec.Code)
	}
}

func TestPatchRenamesAndReordersInOneRequest(t *testing.T) {
	// A drag reorders and the inline editor renames, but the endpoint accepts
	// both at once and applies each.
	reader := &fakeReader{
		updated: domain.Project{ID: "p1", Name: "harbour"},
		list:    []domain.Project{{ID: "p1", Name: "harbour"}},
	}
	r := newRouterWithOps(reader, &fakeImporter{}, &fakeDeleter{}, nil)

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
}
