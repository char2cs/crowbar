package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func newRouter(
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
) *gin.Engine {
	return newRouterWithDeleter(reader, importer, &fakeDeleter{})
}

func newRouterWithDeleter(
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
) *gin.Engine {
	r := gin.New()
	h := projecthandlers.New(reader, importer, deleter)
	rg := r.Group("/v0")
	rg.GET("/projects", h.List)
	rg.POST("/projects", h.Import)
	rg.GET("/projects/:id", h.Detail)
	rg.DELETE("/projects/:id", h.Delete)
	return r
}

func do(
	r *gin.Engine,
	method string,
	target string,
	body string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, target, reader)
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

func TestImportSuccess(
	t *testing.T,
) {
	importer := &fakeImporter{project: domain.Project{ID: "new-id"}}
	rec := do(
		newRouter(&fakeReader{}, importer),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma","path":"/g"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "new-id", body.Data.ID)
	assert.Equal(t, "gamma", importer.gotName)
	assert.Equal(t, "/g", importer.gotPath)
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

func TestDeleteSuccess(
	t *testing.T,
) {
	deleter := &fakeDeleter{}
	rec := do(
		newRouterWithDeleter(&fakeReader{}, &fakeImporter{}, deleter),
		http.MethodDelete,
		"/v0/projects/p1",
		"",
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "p1", body.Data.ID)
	assert.Equal(t, "p1", deleter.gotID)
}

func TestDeleteNotFound(
	t *testing.T,
) {
	deleter := &fakeDeleter{err: apperr.ErrNotFound}
	rec := do(
		newRouterWithDeleter(&fakeReader{}, &fakeImporter{}, deleter),
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

func TestDeleteUsecaseError(
	t *testing.T,
) {
	deleter := &fakeDeleter{err: errors.New("boom")}
	rec := do(
		newRouterWithDeleter(&fakeReader{}, &fakeImporter{}, deleter),
		http.MethodDelete,
		"/v0/projects/p1",
		"",
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestImportUsecaseError(
	t *testing.T,
) {
	importer := &fakeImporter{err: errors.New("boom")}
	rec := do(
		newRouter(&fakeReader{}, importer),
		http.MethodPost,
		"/v0/projects",
		`{"name":"gamma","path":"/g"}`,
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
