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
	"github.com/char2cs/crowbar/api/internal/testutil"
)

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

	if c.Writer.Status() != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", c.Writer.Status())
	}
}

func TestServe404sOnAnOversizeFile(t *testing.T) {
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
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}

func TestValidateRefusesANonImage(t *testing.T) {
	c, rec := ctx(httptest.NewRequest(http.MethodPut, "/", nil))

	if icons.Validate(c, []byte("root:x:0:0:root:/root:/bin/sh\n")) {
		t.Fatal("a text file passed validation")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

const scriptedSVG = `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`

func TestValidateRefusesAnSVGUpload(t *testing.T) {
	c, rec := ctx(httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/", nil))

	if icons.Validate(c, []byte(scriptedSVG)) {
		t.Fatal("an svg upload passed validation")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestServeSandboxesAnSVG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon")
	if err := os.WriteFile(path, []byte(scriptedSVG), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c, rec := ctx(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	icons.Serve(c, path)

	if got := rec.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "sandbox") {
		t.Errorf("Content-Security-Policy = %q, want default-src 'none' and sandbox", csp)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
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

func TestContentTypeOnlySniffsTheFirst512Bytes(t *testing.T) {
	head := strings.Repeat("x", 600)
	if got := icons.ContentType([]byte(head)); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("ContentType(600 bytes of text) = %q", got)
	}
	late := []byte(strings.Repeat("x", 600) + "<svg")
	if got := icons.ContentType(late); got == "image/svg+xml" {
		t.Fatal("sniffed past the 512-byte head")
	}
}

func TestReadUploadRefusesAPathThatCannotBeRead(t *testing.T) {
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
	if icons.IsSingleEmoji("\xff") {
		t.Fatal("invalid UTF-8 was accepted as an emoji")
	}
}

func TestServe404sWhenTheFileCannotBeOpened(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	path := filepath.Join(t.TempDir(), "icon")
	if err := os.WriteFile(path, png(), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	c, _ := ctx(httptest.NewRequest(http.MethodGet, "/", nil))

	icons.Serve(c, path)

	if c.Writer.Status() != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", c.Writer.Status())
	}
}

func TestServe404sWhenTheFileCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	c, _ := ctx(httptest.NewRequest(http.MethodGet, "/", nil))

	icons.Serve(c, dir)

	if c.Writer.Status() != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", c.Writer.Status())
	}
}

func TestReadUploadRefusesAPathWithNoReadPermission(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, png(), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"path":"`+path+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("an unreadable file was accepted as an icon")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}
