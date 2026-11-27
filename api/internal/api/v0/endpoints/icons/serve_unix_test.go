//go:build unix

package icons_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
)

func TestReadUploadRefusesAPathThatGrowsPastTheCapBetweenStatAndRead(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "growing")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer func() { _ = w.Close() }()
		_, _ = w.Write(make([]byte, icons.MaxBytes+2))
	}()
	t.Cleanup(wg.Wait)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/", strings.NewReader(`{"path":"`+fifo+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c, rec := ctx(req)

	if _, ok := icons.ReadUpload(c); ok {
		t.Fatal("a file that grew past the cap after Stat was accepted")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}
