package icons_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
)

// The read/validate/store half of the icon plumbing, which both the repo and
// the project icon routes run through. Every branch here is a REFUSAL, and the
// refusals are the point: this package is the one place that decides whether a
// client-supplied path or upload becomes bytes on disk.

func png() []byte { return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...) }

func ctx(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
}

func TestServe404sOnAMissingFile(t *testing.T) {
	c, _ := ctx(httptest.NewRequest(http.MethodGet, "/", nil))

	icons.Serve(c, filepath.Join(t.TempDir(), "nope"))

	// c.Writer.Status(), not rec.Code: gin buffers the status until a body write
	// or WriteHeaderNow, and a bodiless 404 writes neither.
	if c.Writer.Status() != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", c.Writer.Status())
	}
}

func TestServe404sOnAnOversizeFile(t *testing.T) {
	// These bytes are written by this daemon, but a corrupted or replaced file
	// must not become an unbounded heap allocation.
	path := filepath.Join(t.TempDir(), "icon")
	if err := os.WriteFile(path, make([]byte, icons.MaxBytes+1), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c, _ := ctx(httptest.NewRequest(http.MethodGet, "/", nil))

	icons.Serve(c, path)

	if c.Writer.Status() != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", c.Writer.Status())
	}
}

func TestServeSendsTheBytesWithNoCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon")
	if err := os.WriteFile(path, png(), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c, rec := ctx(httptest.NewRequest(http.MethodGet, "/", nil))

	icons.Serve(c, path)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
	// The bytes change in place behind a stable URL, so the response must not be
	// cacheable on its own; the DTO's ?v= is the primary buster.
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}

func TestValidateRefusesANonImage(t *testing.T) {
	// By SNIFFING, never by the extension or the caller's Content-Type: a .png
	// filename over /etc/passwd must not become an icon.
	c, rec := ctx(httptest.NewRequest(http.MethodPut, "/", nil))

	if icons.Validate(c, []byte("root:x:0:0:root:/root:/bin/sh\n")) {
		t.Fatal("a text file passed validation")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestValidateRefusesAnOversizeImage(t *testing.T) {
	c, rec := ctx(httptest.NewRequest(http.MethodPut, "/", nil))

	if icons.Validate(c, append(png(), make([]byte, icons.MaxBytes)...)) {
		t.Fatal("an oversize image passed validation")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestValidateAcceptsARealImage(t *testing.T) {
	c, _ := ctx(httptest.NewRequest(http.MethodPut, "/", nil))

	if !icons.Validate(c, png()) {
		t.Fatal("a png was refused")
	}
}

func TestStoreCreatesTheParentDirectory(t *testing.T) {
	// The entity's icon directory does not exist until its first upload.
	path := filepath.Join(t.TempDir(), "projects", "p1", "icon")

	if err := icons.Store(path, png()); err != nil {
		t.Fatalf("store: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, png()) {
		t.Fatal("stored bytes differ from the ones written")
	}
}

func TestStoreSurfacesAnUnwritableDestination(t *testing.T) {
	// A file where the directory should be: MkdirAll cannot proceed.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := icons.Store(filepath.Join(blocker, "icon"), png()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestReadUploadTakesAMultipartField(t *testing.T) {
	// The browser path. The desktop one sends a JSON {path} instead, because the
	// WKWebView crowbar:// transport cannot carry a multipart body.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("icon", "logo.png")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(png()); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	c, _ := ctx(req)

	data, ok := icons.ReadUpload(c)

	if !ok {
		t.Fatal("multipart upload was refused")
	}
	if !bytes.Equal(data, png()) {
		t.Fatal("read bytes differ from the uploaded ones")
	}
}

func TestReadUploadRefusesAMultipartWithNoIconField(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("something-else", "x"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("expected a refusal")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestReadUploadRefusesAJSONBodyWithNoPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("expected a refusal")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestReadUploadRefusesAPathThatIsNotThere(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/",
		strings.NewReader(`{"path":"`+filepath.Join(t.TempDir(), "missing")+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("expected a refusal")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestReadUploadRefusesAnOversizeFileBeforeReadingIt(t *testing.T) {
	// Stat-rejected: the point is that an enormous file is never read into the
	// heap just to find out it is too big.
	path := filepath.Join(t.TempDir(), "huge.png")
	if err := os.WriteFile(path, make([]byte, icons.MaxBytes+1), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"path":"`+path+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("expected a refusal")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestDefaultCrowbarHomeHonoursTheEnvOverride(t *testing.T) {
	// Dev instances point this inside the workspace being developed, which is
	// the whole of their isolation from a real install.
	t.Setenv("CROWBAR_HOME", "/tmp/crowbar-test-home")

	got, err := icons.DefaultCrowbarHome()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if got != "/tmp/crowbar-test-home" {
		t.Fatalf("home = %q, want the override", got)
	}
}

func TestContentTypeOnlySniffsTheFirst512Bytes(t *testing.T) {
	// The SVG probe reads a HEAD, not the whole file: a multi-megabyte asset
	// must not be scanned end to end on every request that serves it.
	head := strings.Repeat("x", 600)
	if got := icons.ContentType([]byte(head)); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("ContentType(600 bytes of text) = %q", got)
	}
	// An <svg> past the 512-byte window is deliberately NOT found.
	late := []byte(strings.Repeat("x", 600) + "<svg")
	if got := icons.ContentType(late); got == "image/svg+xml" {
		t.Fatal("sniffed past the 512-byte head")
	}
}

func TestReadUploadRefusesAPathThatCannotBeRead(t *testing.T) {
	// A directory stats fine and opens fine on Unix; the READ is what fails. It
	// is the last of the three guards on this path, and the only one that
	// catches a path which looked plausible right up to the bytes.
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"path":"`+t.TempDir()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("a directory was accepted as an icon")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestIsSingleEmojiRefusesInvalidUTF8(t *testing.T) {
	// A lone continuation byte decodes to RuneError. It is one grapheme, so the
	// cluster count alone would let it through and store a broken label.
	if icons.IsSingleEmoji("\xff") {
		t.Fatal("invalid UTF-8 was accepted as an emoji")
	}
}
