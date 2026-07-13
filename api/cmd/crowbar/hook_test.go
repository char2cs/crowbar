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

func TestRunHook_ForwardsSegmentProviderAndRawPayload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "hook")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	sock := filepath.Join(tmpDir, "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var gotPath string
	var got map[string]any
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&got)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runHook("turn_stop", "seg-42", "claude", "p1", "r1", "w1", []byte(`{"session_id":"abc"}`), "unix://"+sock)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "/v0/projects/p1/repos/r1/workspaces/w1/agent/hooks", gotPath)
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "claude", got["provider"])
	require.Equal(t, "turn_stop", got["event"])
	require.Equal(t, `{"session_id":"abc"}`, got["payload_raw"])
}

func TestResolvePayload_Precedence(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.WriteFile(f, []byte("FROMFILE"), 0o644))

	inline, err := resolvePayload("INLINE", f, strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "INLINE", string(inline))

	fromFile, err := resolvePayload("", f, strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "FROMFILE", string(fromFile))

	fromStdin, err := resolvePayload("", "", strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "FROMSTDIN", string(fromStdin))
}
