package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
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
	got, err := os.ReadFile(stored) //nolint:gosec // G304: test-owned temp path.
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
