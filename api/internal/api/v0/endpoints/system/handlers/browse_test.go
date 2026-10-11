package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBrowse_ListsDirectory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "zeta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/browse?path="+dir, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Dirs first (alpha, zeta), then files (readme.txt).
	body := rec.Body.String()
	alphaIdx, zetaIdx, readmeIdx := indexOf(body, `"alpha"`), indexOf(body, `"zeta"`), indexOf(body, `"readme.txt"`)
	if alphaIdx >= zetaIdx || zetaIdx >= readmeIdx {
		t.Fatalf("expected dirs-first, alpha-before-zeta ordering; got body: %s", body)
	}
}

func TestBrowse_NonexistentPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/browse?path=/definitely/not/a/real/path/xyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a 4xx for a nonexistent path, got %d", rec.Code)
	}
}

func TestBrowse_PathIsAFile_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	file := filepath.Join(dir, "notadir.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/browse?path="+file, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a 4xx for a file path, got %d", rec.Code)
	}
}

func TestBrowse_EmptyPath_DefaultsToHome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/browse", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty path (home default), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRegression_Browse_RootChildPathsAreJoined proves browsing the
// filesystem root yields "/etc", not "//etc": child paths are joined, not
// concatenated with a separator the parent already ends in.
func TestRegression_Browse_RootChildPathsAreJoined(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/browse?path="+url.QueryEscape(root), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for the root, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data []BrowseEntry `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) == 0 {
		t.Fatal("expected the root to list at least one entry")
	}
	for _, e := range body.Data {
		if want := filepath.Join(root, e.Name); e.Path != want {
			t.Fatalf("entry %q: path %q, want %q", e.Name, e.Path, want)
		}
	}
}

// TestBrowse_RelativePath_Returns400 proves a relative path is refused rather
// than resolved against whatever the daemon's working directory happens to be.
func TestBrowse_RelativePath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/browse?path=.", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a relative path, got %d: %s", rec.Code, rec.Body.String())
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
