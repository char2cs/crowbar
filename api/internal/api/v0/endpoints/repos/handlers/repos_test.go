package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeStore struct {
	all      []domain.Repository
	allErr   error
	byKey    *domain.Repository
	byKeErr  error
	SaveFn   func(ctx context.Context, r domain.Repository) error
	DeleteFn func(ctx context.Context, id string) error
}

func (f *fakeStore) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	return f.all, f.allErr
}

func (f *fakeStore) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	return f.byKey, f.byKeErr
}

func (f *fakeStore) Save(
	ctx context.Context,
	r domain.Repository,
) error {
	if f.SaveFn != nil {
		return f.SaveFn(ctx, r)
	}
	return nil
}

func (f *fakeStore) Delete(
	ctx context.Context,
	id string,
) error {
	if f.DeleteFn != nil {
		return f.DeleteFn(ctx, id)
	}
	return nil
}

func newRouter(
	store repohandlers.Store,
) *gin.Engine {
	r := gin.New()
	h := repohandlers.New(store)
	rg := r.Group("/v0")
	rg.GET("/repos", h.List)
	rg.GET("/repos/:id", h.Detail)
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

func TestListSuccess(
	t *testing.T,
) {
	store := &fakeStore{
		all: []domain.Repository{
			{ID: "r1", ProjectID: "p1", Name: "alpha"},
			{ID: "r2", ProjectID: "p2", Name: "beta"},
		},
	}
	rec := do(newRouter(store), "/v0/repos")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Len(t, body.Data, 2)
}

func TestListFilteredByProject(
	t *testing.T,
) {
	store := &fakeStore{
		all: []domain.Repository{
			{ID: "r1", ProjectID: "p1"},
			{ID: "r2", ProjectID: "p2"},
		},
	}
	rec := do(newRouter(store), "/v0/repos?projectId=p2")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "r2", body.Data[0].ID)
}

func TestListError(
	t *testing.T,
) {
	rec := do(newRouter(&fakeStore{allErr: errors.New("db down")}), "/v0/repos")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDetailSuccess(
	t *testing.T,
) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r9", Name: "gamma"}}
	rec := do(newRouter(store), "/v0/repos/r9")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "r9", body.Data.ID)
}

func TestDetailNotFound(
	t *testing.T,
) {
	rec := do(newRouter(&fakeStore{byKey: nil}), "/v0/repos/missing")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.NotEmpty(t, body.Error)
}

func TestDetailError(
	t *testing.T,
) {
	rec := do(newRouter(&fakeStore{byKeErr: errors.New("boom")}), "/v0/repos/r1")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// iconRouter mounts the icon route with the hierarchical :projectId/:repoId
// params and wires the handler's icon storage to a temp home + an optional
// avatar-bytes fetcher.
func iconRouter(
	store repohandlers.Store,
	home string,
	fetch repohandlers.AvatarBytesFetcher,
	method string,
	path string,
) *gin.Engine {
	h := repohandlers.New(store).WithIconStorage(
		func() (string, error) { return home, nil },
		fetch,
	)
	r := gin.New()
	switch method {
	case http.MethodGet:
		r.GET(path, h.Icon)
	case http.MethodPut:
		if strings.HasSuffix(path, "/icon/github") {
			r.PUT(path, h.PutIconGithub)
		} else {
			r.PUT(path, h.PutIconEmoji)
		}
	case http.MethodDelete:
		r.DELETE(path, h.DeleteIcon)
	}
	return r
}

const iconRoute = "/v0/projects/:projectId/repos/:repoId/icon"

func TestIcon_ServesOnDiskBytes(t *testing.T) {
	home := t.TempDir()
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	// PNG magic bytes so http.DetectContentType reports image/png.
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("rest")...)
	require.NoError(t, os.WriteFile(iconPath, png, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, png, w.Body.Bytes())
}

func TestIcon_NoIcon_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: false}}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

type fakeBranchProvider struct {
	protected []string
}

func (f *fakeBranchProvider) ProtectedBranches(_ context.Context, _ string) ([]string, error) {
	return f.protected, nil
}

type fakeWSReader struct {
	workspaces []domain.Workspace
}

func (f *fakeWSReader) List(_ context.Context) ([]domain.Workspace, error) {
	return f.workspaces, nil
}

func TestBranches_AnnotatesProtectionAndWorkspace(t *testing.T) {
	// Note: git branch -r uses real git, so we can't easily test it in unit tests.
	// We test the handler returns 404 for a missing repo.
	h := repohandlers.NewWithDeps(
		&fakeStore{byKey: nil},
		&fakeBranchProvider{protected: []string{"main"}},
		&fakeWSReader{},
		nil,
	)
	r := gin.New()
	r.GET("/v0/repos/:id/branches", h.Branches)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/repos/r1/branches", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPutIconEmoji_SetsEmoji(t *testing.T) {
	home := t.TempDir()
	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo", AvatarHasIcon: true}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error {
		saved = r
		return nil
	}
	r := iconRouter(store, home, nil, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{"emoji":"🦊"}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "🦊", saved.AvatarEmoji)
	assert.False(t, saved.AvatarHasIcon, "setting an emoji clears the on-disk icon flag")
}

func TestPutIconEmoji_MissingRepo_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: nil}
	r := iconRouter(store, home, nil, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{"emoji":"🦊"}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/missing/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteIcon_ClearsFlag(t *testing.T) {
	home := t.TempDir()
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	require.NoError(t, os.WriteFile(iconPath, []byte("img"), 0o644))

	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true, AvatarEmoji: "🦊"}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error {
		saved = r
		return nil
	}
	r := iconRouter(store, home, nil, http.MethodDelete, iconRoute)

	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, saved.AvatarHasIcon)
	assert.Empty(t, saved.AvatarEmoji)
	_, statErr := os.Stat(iconPath)
	assert.True(t, os.IsNotExist(statErr), "the on-disk icon file must be removed")
}

func TestPutIconGithub_StoresBytes(t *testing.T) {
	home := t.TempDir()
	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error {
		saved = r
		return nil
	}
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return []byte("AVATARBYTES"), "image/png", nil
	}
	r := iconRouter(store, home, fetch, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, saved.AvatarHasIcon)

	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	data, readErr := os.ReadFile(iconPath)
	require.NoError(t, readErr)
	assert.Equal(t, "AVATARBYTES", string(data))
}

func TestPutIconGithub_FetchFails_Returns422(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return nil, "", nil // degraded
	}
	r := iconRouter(store, home, fetch, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
