package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunHook_ForwardsSegmentAndPayload(t *testing.T) {
	// Use a short temp directory to stay under macOS's 104-byte sun_path
	// limit (t.TempDir() nests under the long test name and overflows it).
	tmpDir, err := os.MkdirTemp("/tmp", "hook")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	sock := filepath.Join(tmpDir, "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var got map[string]any
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&got)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	t.Setenv("CROWBAR_SEGMENT_ID", "seg-42")
	err = runHook("session_start", strings.NewReader(`{"session_id":"abc","source":"startup"}`), "unix://"+sock)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "session_start", got["event"])
	payload := got["payload"].(map[string]any)
	require.Equal(t, "abc", payload["session_id"])
}
