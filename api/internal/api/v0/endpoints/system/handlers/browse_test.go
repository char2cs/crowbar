package handlers

import (
	"net/http"
	"net/http/httptest"
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
	req := httptest.NewRequest(http.MethodGet, "/browse?path="+dir, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Dirs first (alpha, zeta), then files (readme.txt).
	body := rec.Body.String()
	alphaIdx, zetaIdx, readmeIdx := indexOf(body, `"alpha"`), indexOf(body, `"zeta"`), indexOf(body, `"readme.txt"`)
	if !(alphaIdx < zetaIdx && zetaIdx < readmeIdx) {
		t.Fatalf("expected dirs-first, alpha-before-zeta ordering; got body: %s", body)
	}
}

func TestBrowse_NonexistentPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/browse", Browse)
	req := httptest.NewRequest(http.MethodGet, "/browse?path=/definitely/not/a/real/path/xyz", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/browse?path="+file, nil)
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
	req := httptest.NewRequest(http.MethodGet, "/browse", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty path (home default), got %d: %s", rec.Code, rec.Body.String())
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
