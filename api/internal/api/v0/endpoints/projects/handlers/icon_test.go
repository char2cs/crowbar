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

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The project icon endpoints, which mirror the repo ones a level up. What the
// tests below pin is the part that is easy to get wrong twice: an image and an
// emoji are ONE choice with three states, so setting either must clear the
// other, and "reset" must clear both — otherwise a project that was given an
// image over an emoji shows the emoji again the moment the image is removed.
//
// There is deliberately no /icon/github here. A project has no origin remote to
// read an owner avatar from; see icon.go.

// pngBytes is the smallest thing http.DetectContentType calls image/png.
func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
}

// iconRouter wires the icon routes against a temp crowbar home, returning the
// router, the reader the assertions read back through, and that home.
func iconRouter(t *testing.T) (*gin.Engine, *fakeReader, string) {
	t.Helper()
	home := t.TempDir()
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour"}}
	h := projecthandlers.
		New(reader, &fakeImporter{}, &fakeDeleter{}, newRecordingBroadcaster().push).
		WithIconStorage(func() (string, error) { return home, nil })

	r := gin.New()
	rg := r.Group("/v0")
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	rg.PUT("/projects/:projectId/icon/emoji", h.PutIconEmoji)
	return r, reader, home
}

// putIconFromPath drives the desktop upload path: a JSON body naming a file the
// daemon reads itself, because the WKWebView transport cannot carry multipart.
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

func TestPutIconRejectsANonImage(t *testing.T) {
	// Content sniffing, not the extension: a .png name over text must not be
	// stored, or a JSON path could hand the daemon any file on the host.
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
	// The read path gates on the project's own flag, which the fake reader
	// answers from `get` rather than from the Update it recorded.
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

// The paths that refuse. Each one is a branch that only runs when something has
// already gone wrong, which is exactly when a silent fallthrough would write an
// icon nobody can serve.

// brokenHomeRouter wires the icon routes with a crowbar-home resolver that
// fails, standing in for an install whose state root cannot be resolved.
func brokenHomeRouter(reader *fakeReader) *gin.Engine {
	h := projecthandlers.
		New(reader, &fakeImporter{}, &fakeDeleter{}, newRecordingBroadcaster().push).
		WithIconStorage(func() (string, error) { return "", errNoHome })

	r := gin.New()
	rg := r.Group("/v0")
	rg.GET("/projects/:projectId/icon", h.Icon)
	rg.PUT("/projects/:projectId/icon", h.PutIcon)
	rg.DELETE("/projects/:projectId/icon", h.DeleteIcon)
	return r
}

var errNoHome = errors.New("no crowbar home")

func TestIconRoutes404WhenTheProjectIsGone(t *testing.T) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
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
	// Better a 500 than an upload that reports success and stores nothing.
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
	// The file cannot be removed, but the flags must still clear — otherwise the
	// project keeps claiming an icon the serve path will 404 on forever.
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
	reader.updateErr = apperr.ErrNotFound

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/icon/emoji", strings.NewReader(`{"emoji":"🦊"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("PUT emoji = %d, want a 4xx/5xx when the store refuses", rec.Code)
	}
}

func TestGetIcon404sWhenTheHomeCannotBeResolved(t *testing.T) {
	// The project claims an icon but the state root is gone: 404 rather than a
	// half-served response.
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour", AvatarHasIcon: true}}

	rec := httptest.NewRecorder()
	brokenHomeRouter(reader).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/projects/p1/icon", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET icon = %d, want 404", rec.Code)
	}
}

func TestIconMutations404OnAnUnknownProject(t *testing.T) {
	// Every mutation checks the project exists before touching bytes or store.
	r, reader, _ := iconRouter(t)
	reader.getErr = apperr.ErrNotFound

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
	// ReadUpload writes its own 4xx and returns ok=false; the handler must stop
	// there rather than storing an empty file.
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
	// A file where the project's icon DIRECTORY should be: the store cannot
	// create the parent, and the handler must say so rather than record an icon
	// that was never written.
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "projects", "p1"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	reader := &fakeReader{get: domain.Project{ID: "p1", Name: "harbour"}}
	h := projecthandlers.
		New(reader, &fakeImporter{}, &fakeDeleter{}, newRecordingBroadcaster().push).
		WithIconStorage(func() (string, error) { return home, nil })
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
