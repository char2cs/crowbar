package handlers_test

import (
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

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
}

func iconRouter(t *testing.T) (*gin.Engine, *fakeReader, string) {
	t.Helper()
	home := t.TempDir()
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour"}}
	h := projecthandlers.
		New(reader, &fakeImporter{}, &fakeDeleter{}, nil, func() (string, error) { return home, nil })

	r := gin.New()
	rg := r.Group("/v0")
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	rg.PUT("/projects/:projectId/icon/emoji", h.PutIconEmoji)
	return r, reader, home
}

func putIconFromPath(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestPutIconStoresBytesAndClearsEmoji(t *testing.T) {
	r, reader, home := iconRouter(t)
	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if rec := putIconFromPath(t, r, src); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT icon = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}

	stored := filepath.Join(home, "projects", "p1", "icon")
	got, err := os.ReadFile(stored)
	if err != nil {
		t.Fatalf("icon not stored at %s: %v", stored, err)
	}
	if string(got) != string(pngBytes()) {
		t.Fatalf("stored bytes differ from the uploaded ones")
	}
	if reader.updatedWith == nil {
		t.Fatal("handler never persisted the icon change")
	}
	if reader.updatedWith.AvatarHasIcon == nil || !*reader.updatedWith.AvatarHasIcon {
		t.Error("AvatarHasIcon was not set")
	}
	if reader.updatedWith.AvatarEmoji == nil || *reader.updatedWith.AvatarEmoji != "" {
		t.Error("an uploaded image must clear any emoji")
	}
	if !reader.updatedWith.BumpAvatarVersion {
		t.Error("new bytes behind a stable URL must bump the cache-busting version")
	}
}

func TestPutIconAWriteFailureNamesThePath(t *testing.T) {
	r, _, home := iconRouter(t)
	stored := filepath.Join(home, "projects", "p1", "icon")
	if err := os.MkdirAll(stored, 0o750); err != nil {
		t.Fatalf("block the icon path: %v", err)
	}
	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	rec := putIconFromPath(t, r, src)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT icon = %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
	assert.JSONEq(t, `{"success":false,"code":"internal","error":"write error: open `+stored+`: is a directory"}`, rec.Body.String())
}

func TestPutIconRejectsANonImage(t *testing.T) {
	r, _, home := iconRouter(t)
	src := filepath.Join(t.TempDir(), "passwd.png")
	if err := os.WriteFile(src, []byte("root:x:0:0:root:/root:/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if rec := putIconFromPath(t, r, src); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT icon = %d, want 400", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(home, "projects", "p1", "icon")); !os.IsNotExist(err) {
		t.Fatal("a rejected upload must leave nothing on disk")
	}
}

func TestPutIconEmojiClearsTheStoredImage(t *testing.T) {
	r, reader, home := iconRouter(t)
	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	putIconFromPath(t, r, src)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon/emoji", strings.NewReader(`{"emoji":"🦊"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT emoji = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if reader.updatedWith.AvatarEmoji == nil || *reader.updatedWith.AvatarEmoji != "🦊" {
		t.Error("emoji was not persisted")
	}
	if reader.updatedWith.AvatarHasIcon == nil || *reader.updatedWith.AvatarHasIcon {
		t.Error("an emoji must clear the on-disk icon flag")
	}
	if _, err := os.Stat(filepath.Join(home, "projects", "p1", "icon")); !os.IsNotExist(err) {
		t.Error("an emoji must remove the image file it replaced")
	}
}

func TestPutIconEmojiRejectsMoreThanOne(t *testing.T) {
	r, _, _ := iconRouter(t)
	for _, body := range []string{`{"emoji":"🦊🦊"}`, `{"emoji":"ab"}`, `{"emoji":""}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon/emoji", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT emoji %s = %d, want 400", body, rec.Code)
		}
	}
}

func TestDeleteIconClearsBothStates(t *testing.T) {
	r, reader, home := iconRouter(t)
	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	putIconFromPath(t, r, src)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/icon", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE icon = %d, want 204", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(home, "projects", "p1", "icon")); !os.IsNotExist(err) {
		t.Error("reset must remove the stored image")
	}
	if reader.updatedWith.AvatarHasIcon == nil || *reader.updatedWith.AvatarHasIcon {
		t.Error("reset must clear the on-disk icon flag")
	}
	if reader.updatedWith.AvatarEmoji == nil || *reader.updatedWith.AvatarEmoji != "" {
		t.Error("reset must clear the emoji too — otherwise it re-appears under the removed image")
	}
}

func TestGetIconServesTheStoredBytes(t *testing.T) {
	r, reader, _ := iconRouter(t)
	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	putIconFromPath(t, r, src)
	reader.get = domain.Project{ID: "p1", Name: "harbour", AvatarHasIcon: true}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/projects/p1/icon", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET icon = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if rec.Body.String() != string(pngBytes()) {
		t.Error("served bytes differ from the stored ones")
	}
}

func TestGetIconIs404WithoutOne(t *testing.T) {
	r, _, _ := iconRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/projects/p1/icon", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET icon = %d, want 404", rec.Code)
	}
}

func brokenHomeRouter(reader *fakeReader) *gin.Engine {
	h := projecthandlers.
		New(reader, &fakeImporter{}, &fakeDeleter{}, nil, unusedHome)

	r := gin.New()
	rg := r.Group("/v0")
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	return r
}

var errNoHome = errors.New("no crowbar home")

func unusedHome() (string, error) { return "", errNoHome }

func TestIconRoutes404WhenTheProjectIsGone(t *testing.T) {
	reader := &fakeReader{getErr: problem.ErrNotFound}
	broken := brokenHomeRouter(reader)

	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/v0/projects/p1/icon"},
		{http.MethodDelete, "/v0/projects/p1/icon"},
	} {
		rec := httptest.NewRecorder()
		broken.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.target, rec.Code)
		}
	}
}

func TestPutIconFailsClosedWithNoResolvableHome(t *testing.T) {
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour"}}
	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	body, err := json.Marshal(map[string]string{"path": src})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	brokenHomeRouter(reader).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT icon = %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
	if reader.updatedWith != nil {
		t.Error("nothing may be persisted when the bytes could not be stored")
	}
}

func TestDeleteIconStillClearsTheRecordWithNoResolvableHome(t *testing.T) {
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour", AvatarHasIcon: true}}

	rec := httptest.NewRecorder()
	brokenHomeRouter(reader).ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/icon", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE icon = %d, want 204", rec.Code)
	}
	if reader.updatedWith == nil || reader.updatedWith.AvatarHasIcon == nil || *reader.updatedWith.AvatarHasIcon {
		t.Error("the on-disk icon flag must be cleared regardless")
	}
}

func TestIconMutationsSurfaceAFailedWrite(t *testing.T) {
	r, reader, _ := iconRouter(t)
	reader.updateErr = problem.ErrNotFound

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon/emoji", strings.NewReader(`{"emoji":"🦊"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("PUT emoji = %d, want a 4xx/5xx when the store refuses", rec.Code)
	}
}

func TestGetIcon404sWhenTheHomeCannotBeResolved(t *testing.T) {
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour", AvatarHasIcon: true}}

	rec := httptest.NewRecorder()
	brokenHomeRouter(reader).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/projects/p1/icon", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET icon = %d, want 404", rec.Code)
	}
}

func TestIconMutations404OnAnUnknownProject(t *testing.T) {
	r, reader, _ := iconRouter(t)
	reader.getErr = problem.ErrNotFound

	cases := []struct {
		name, method, target, body, ctype string
	}{
		{"put image", http.MethodPut, "/v0/projects/p1/icon", `{"path":"/nope"}`, "application/json"},
		{"put emoji", http.MethodPut, "/v0/projects/p1/icon/emoji", `{"emoji":"🦊"}`, "application/json"},
		{"delete", http.MethodDelete, "/v0/projects/p1/icon", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", tc.ctype)
			} else {
				req = httptest.NewRequest(tc.method, tc.target, nil)
			}
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s = %d, want 404", tc.name, rec.Code)
			}
			if reader.updatedWith != nil {
				t.Error("an unknown project must never reach the store")
			}
		})
	}
}

func TestPutIconRefusesAnUnreadableUpload(t *testing.T) {
	r, reader, home := iconRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT icon = %d, want 400", rec.Code)
	}
	if reader.updatedWith != nil {
		t.Error("nothing may be persisted for an upload that could not be read")
	}
	if _, err := os.Stat(filepath.Join(home, "projects", "p1", "icon")); !os.IsNotExist(err) {
		t.Error("nothing may be written for an upload that could not be read")
	}
}

func TestPutIconSurfacesAFailedWriteToDisk(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "projects", "p1"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour"}}
	h := projecthandlers.
		New(reader, &fakeImporter{}, &fakeDeleter{}, nil, func() (string, error) { return home, nil })
	r := gin.New()
	r.PUT("/v0/projects/:projectId/icon", h.PutIcon)

	src := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(src, pngBytes(), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon",
		strings.NewReader(`{"path":"`+src+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT icon = %d, want 500", rec.Code)
	}
	if reader.updatedWith != nil {
		t.Error("an icon that could not be written must not be recorded as present")
	}
}
