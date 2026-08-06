package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
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

// putIconHandler builds a router wired to the multipart/JSON upload handler.
func putIconRouter(store repohandlers.Store, home string) *gin.Engine {
	h := repohandlers.New(store).WithIconStorage(func() (string, error) { return home, nil }, nil)
	r := gin.New()
	r.PUT(iconRoute, h.PutIcon)
	return r
}

// Desktop transport: the WKWebView crowbar:// scheme cannot carry a
// multipart/binary body, so the native file dialog hands the daemon an absolute
// path (as JSON) and the daemon reads the file itself — exactly like repo import.
func TestPutIcon_JSONPath_ReadsFileFromDisk(t *testing.T) {
	home := t.TempDir()
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("custom-photo-bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarEmoji: "🚀"}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error { saved = r; return nil }

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	putIconRouter(store, home).ServeHTTP(w, req)

	// 204 (not a raw 200 body): the FE apiFetch throws on any non-enveloped
	// 200, so PutIcon must answer 204 like the other icon mutations — the
	// updated avatar arrives on the repos WS stream, not in this response.
	assert.Equal(t, http.StatusNoContent, w.Code)
	stored, err := os.ReadFile(filepath.Join(home, "projects", "p1", "r1", "icon"))
	require.NoError(t, err)
	assert.Equal(t, png, stored)
	assert.True(t, saved.AvatarHasIcon)
	assert.Equal(t, "", saved.AvatarEmoji)
}

func TestPutIcon_JSONPath_MissingFile_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	body, _ := json.Marshal(map[string]string{"path": filepath.Join(home, "does-not-exist.png")})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	putIconRouter(store, home).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutIcon_JSONPath_NonImageExt_Returns400(t *testing.T) {
	home := t.TempDir()
	srcPath := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("not an image"), 0o644))
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	putIconRouter(store, home).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Web browsers still POST the file as multipart/form-data; keep that path working.
func TestPutIcon_Multipart_StoresBytes(t *testing.T) {
	home := t.TempDir()
	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error { saved = r; return nil }

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="icon"; filename="x.png"`},
		"Content-Type":        []string{"image/png"},
	})
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("multipart-bytes")...)
	_, _ = part.Write(png)
	_ = mw.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	putIconRouter(store, home).ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	stored, err := os.ReadFile(filepath.Join(home, "projects", "p1", "r1", "icon"))
	require.NoError(t, err)
	assert.Equal(t, png, stored)
	assert.True(t, saved.AvatarHasIcon)
}

type fakeBranchProvider struct {
	protected []string
	prLinks   []providertypes.PRLink
}

func (f *fakeBranchProvider) ProtectedBranches(_ context.Context, _ string) ([]string, error) {
	return f.protected, nil
}

func (f *fakeBranchProvider) OpenPullRequests(_ context.Context, _ string) ([]providertypes.PRLink, error) {
	return f.prLinks, nil
}

type fakeWSReader struct {
	workspaces []domain.Workspace
	deleted    []string
}

func (f *fakeWSReader) List(_ context.Context) ([]domain.Workspace, error) {
	return f.workspaces, nil
}

func (f *fakeWSReader) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	// A FRESH slice: List hands out the backing array, so filtering in place
	// would mutate the snapshot a caller is still ranging over.
	kept := make([]domain.Workspace, 0, len(f.workspaces))
	for _, w := range f.workspaces {
		if w.ID != id {
			kept = append(kept, w)
		}
	}
	f.workspaces = kept
	return nil
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

func TestPullRequests_ReturnsOpenPRGraph(t *testing.T) {
	h := repohandlers.NewWithDeps(
		&fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}},
		&fakeBranchProvider{prLinks: []providertypes.PRLink{
			{Head: "feat/9324", Base: "feat/base"},
			{Head: "feat/base", Base: "dev"},
		}},
		&fakeWSReader{},
		nil,
	)
	r := gin.New()
	r.GET("/v0/repos/:repoId/pull-requests", h.PullRequests)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/repos/r1/pull-requests", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"head":"feat/9324"`)
	assert.Contains(t, w.Body.String(), `"base":"feat/base"`)
	assert.Contains(t, w.Body.String(), `"base":"dev"`)
}

func TestPullRequests_MissingRepo404(t *testing.T) {
	h := repohandlers.NewWithDeps(&fakeStore{byKey: nil}, &fakeBranchProvider{}, &fakeWSReader{}, nil)
	r := gin.New()
	r.GET("/v0/repos/:repoId/pull-requests", h.PullRequests)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/repos/r1/pull-requests", nil)
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

// TestRegression_RepoIcon_RejectsOversizeOrNonImage asserts that the icon
// upload path:
//   - rejects a file whose size on disk exceeds maxIconBytes (2 MiB), and
//   - rejects a file whose content does not sniff as image/* even when its
//     filename extension is .png.
//
// Both vectors were previously exploitable: an oversize file was read entirely
// into memory before the size check ran, and a non-image file was accepted when
// the caller supplied a .png path because the content-type was derived from the
// extension rather than from content sniffing.
func TestRegression_RepoIcon_RejectsOversizeOrNonImage(t *testing.T) {
	t.Run("oversize file via JSON path is rejected with 400", func(t *testing.T) {
		home := t.TempDir()
		srcPath := filepath.Join(t.TempDir(), "big.png")
		// Write a file of exactly maxIconBytes+1 bytes (2 MiB + 1).
		// maxIconBytes = 2<<20 = 2097152; we write 2097153 bytes.
		oversize := make([]byte, (2<<20)+1)
		// Give it PNG magic bytes so it would pass content sniffing.
		copy(oversize, []byte("\x89PNG\r\n\x1a\n"))
		require.NoError(t, os.WriteFile(srcPath, oversize, 0o644))

		store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
		h := repohandlers.New(store).WithIconStorage(func() (string, error) { return home, nil }, nil)
		r := gin.New()
		r.PUT("/v0/projects/:projectId/repos/:repoId/icon", h.PutIcon)

		body, _ := json.Marshal(map[string]string{"path": srcPath})
		req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "oversize icon must be rejected before the full file is read")
	})

	t.Run("non-image file with .png extension via JSON path is rejected with 400", func(t *testing.T) {
		home := t.TempDir()
		// A file with a .png extension but non-image content (will sniff as text/plain).
		srcPath := filepath.Join(t.TempDir(), "secret.png")
		require.NoError(t, os.WriteFile(srcPath, []byte("root:x:0:0:root:/root:/bin/bash\n"), 0o644))

		store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
		h := repohandlers.New(store).WithIconStorage(func() (string, error) { return home, nil }, nil)
		r := gin.New()
		r.PUT("/v0/projects/:projectId/repos/:repoId/icon", h.PutIcon)

		body, _ := json.Marshal(map[string]string{"path": srcPath})
		req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "non-image content must be rejected by content sniffing even when the extension is .png")
	})
}
