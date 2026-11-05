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

// TestReadUploadRefusesAPathThatGrowsPastTheCapBetweenStatAndRead pins the
// "Hardening applied" guarantee on readFromPath's doc comment: LimitReader
// catches a file that grew larger than MaxBytes AFTER the size-rejecting Stat
// call. A FIFO makes this deterministic without a real race: Stat reports a
// pipe's size as 0 (always under the cap), so the size check passes; the
// actual bytes read past the writer's other end are what trip the post-read
// oversize guard. Opening one end of a FIFO blocks until the other end opens,
// so the writer goroutine's ordering relative to ReadUpload never matters —
// there is no interval to guess at and nothing to re-sample.
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
		// One byte past MaxBytes+1 so the LimitReader-capped read still comes
		// back oversize regardless of how many bytes actually land.
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
