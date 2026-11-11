package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asynxmodels "github.com/char2cs/asynx/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestIcon_ServesFromTheInjectedCrowbarHome pins that icon storage is rooted at
// the home the constructor was given, never at a home the handler resolves
// itself.
func TestIcon_ServesFromTheInjectedCrowbarHome(t *testing.T) {
	devHome := t.TempDir()

	iconPath := filepath.Join(devHome, "projects", "probe-project", "probe-repo", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("dev-home-probe")...)
	require.NoError(t, os.WriteFile(iconPath, png, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "probe-repo", ProjectID: "probe-project", AvatarHasIcon: true}}
	h := repohandlers.New(store, func() (string, error) { return devHome, nil })
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos/:repoId/icon", h.Icon)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/probe-project/repos/probe-repo/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Icon must read the file under the injected home")
	assert.Equal(t, png, rec.Body.Bytes())
}

func TestIcon_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIcon_HomeResolutionFails_Returns404(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	h := repohandlers.New(store, func() (string, error) { return "", errors.New("no home") })
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos/:repoId/icon", h.Icon)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIcon_OversizeOnDiskFile_Returns404(t *testing.T) {
	home := t.TempDir()
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	big := make([]byte, (2<<20)+1)
	require.NoError(t, os.WriteFile(iconPath, big, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIcon_FileMissingOnDisk_Returns404(t *testing.T) {
	home := t.TempDir() // AvatarHasIcon=true but no file written at all
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// PutIconEmoji extra error paths
// ---------------------------------------------------------------------------

func TestPutIconEmoji_MultiCharString_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{"emoji":"not-an-emoji"}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIconEmoji_EmptyBody_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIconEmoji_BadJSON_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{not-json`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIconEmoji_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{"emoji":"🦊"}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A save the store refused for a full queue is a busy daemon, not a crash.
func TestPutIconEmoji_AFullQueueIsUnavailable(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.SaveFn = func(context.Context, domain.Repository) error {
		return fmt.Errorf("repos: save r1: %w", fmt.Errorf("dispatch: %w", asynxmodels.ErrQueueFull))
	}
	r := iconRouter(store, t.TempDir(), nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji",
		strings.NewReader(`{"emoji":"🦊"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"success":false,"code":"unavailable","error":"dispatch: asynx: queue full"}`, rec.Body.String())
}

// ---------------------------------------------------------------------------
// DeleteIcon extra error paths
// ---------------------------------------------------------------------------

func TestDeleteIcon_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := iconRouter(store, home, nil, http.MethodDelete, iconRoute)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteIcon_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }
	r := iconRouter(store, home, nil, http.MethodDelete, iconRoute)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDeleteIcon_HomeResolutionFails_StillClearsFlag(t *testing.T) {
	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true, AvatarEmoji: "🦊"}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error { saved = r; return nil }
	h := repohandlers.New(store, func() (string, error) { return "", errors.New("no home") })
	r := gin.New()
	r.DELETE("/v0/projects/:projectId/repos/:repoId/icon", h.DeleteIcon)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, saved.AvatarHasIcon)
	assert.Empty(t, saved.AvatarEmoji)
}

// ---------------------------------------------------------------------------
// PutIcon / readIconFromPath / readIconFromMultipart / storeIconBytes extra
// error paths.
// ---------------------------------------------------------------------------

func TestPutIcon_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := putIconRouter(store, home)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPutIcon_JSONPath_EmptyPath_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	body, _ := json.Marshal(map[string]string{"path": ""})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_JSONPath_BadJSON_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_Multipart_MissingField_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	var buf bytes.Buffer
	// Valid multipart body but no "icon" part.
	buf.WriteString("--X\r\nContent-Disposition: form-data; name=\"other\"\r\n\r\nvalue\r\n--X--\r\n")
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_Multipart_NonImageContent_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	var buf bytes.Buffer
	buf.WriteString("--X\r\nContent-Disposition: form-data; name=\"icon\"; filename=\"x.png\"\r\nContent-Type: image/png\r\n\r\n")
	buf.WriteString("not actually an image, just text bytes")
	buf.WriteString("\r\n--X--\r\n")
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }

	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPutIcon_StoreIconBytesFails_HomeResolutionError_Returns500(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	h := repohandlers.New(store, func() (string, error) { return "", errors.New("no home") })
	r := gin.New()
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon", h.PutIcon)

	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPutIcon_LargerThanMax_ViaMultipart_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	var buf bytes.Buffer
	buf.WriteString("--X\r\nContent-Disposition: form-data; name=\"icon\"; filename=\"big.png\"\r\nContent-Type: image/png\r\n\r\n")
	oversize := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, (2<<20)+16)...)
	buf.Write(oversize)
	buf.WriteString("\r\n--X--\r\n")
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// storeIconBytes: MkdirAll and WriteFile failure branches, exercised through
// PutIcon (the only exported caller of the unexported storeIconBytes).
// ---------------------------------------------------------------------------

func TestPutIcon_MkdirAllFails_Returns500(t *testing.T) {
	home := t.TempDir()
	// Pre-create a *file* (not a directory) at the path storeIconBytes needs to
	// MkdirAll through, so MkdirAll fails with ENOTDIR.
	blocker := filepath.Join(home, "projects", "p1")
	require.NoError(t, os.MkdirAll(filepath.Dir(blocker), 0o755))
	require.NoError(t, os.WriteFile(blocker, []byte("i am a file, not a dir"), 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPutIcon_WriteFileFails_TargetIsDirectory_Returns500(t *testing.T) {
	home := t.TempDir()
	// Pre-create a *directory* at the exact icon file path, so the parent
	// MkdirAll succeeds but the final os.WriteFile fails (can't write a file
	// where a directory already exists).
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(iconPath, 0o755))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ---------------------------------------------------------------------------
// PutIconGithub extra branches not covered by the original suite.
// ---------------------------------------------------------------------------

func TestPutIconGithub_NoLocalPath_Returns422(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: ""}}
	r := iconRouter(store, home, nil, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestPutIconGithub_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := iconRouter(store, home, nil, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPutIconGithub_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return []byte("AVATARBYTES"), "image/png", nil
	}
	r := iconRouter(store, home, fetch, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
